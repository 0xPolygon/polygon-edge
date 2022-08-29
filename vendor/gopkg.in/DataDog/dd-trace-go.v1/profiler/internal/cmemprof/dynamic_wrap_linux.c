#define _GNU_SOURCE
#include <elf.h>
#include <link.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>

// This code traverses the linked libraries at startup to find any references to
// malloc/calloc/etc in their relocation tables. These are not visible at
// compile time and thus need to be edited at runtime to point to our wrappers.
//
// OVERVIEW:
//
// On Linux, executables and shared libraries are stored in the ELF file format
// (specified at https://refspecs.linuxfoundation.org/elf/elf.pdf). We call ELF
// files "objects". An ELF object contains code and other data needed for running
// programs. This information is divided up in two ways:
//
//	* sections, which contain distinct kinds of information such as code,
//	  strings and defined constants, names of libraries to link, etc.
//	* segments, which are regions of the object that must be loaded into
//	  memory to actually run the program or make the shared library's
//	  code and data accessible to other programs and libraries. Segments
//	  can contain many sections, but not all sections go in a segment.
//
// The segments in an ELF object are defined by its program headers. At runtime,
// we can find the program headers for the program and each linked library by
// calling dl_iterate_phdr.
//
// The segment we want is PT_DYNAMIC. This segment contains references to various
// other parts of the ELF object needed as part of dynamic linking, including:
//
//	* a (dynamic) symbol table that specifies functions and other data
//	  that the object either contains (and can provide to other objects)
//	  or needs from another object.
//	* a string table. Symbols in the symbol table have names, and the names
//	  are contained in the string table.
//	* relocation tables. There are several varieties, but essentially all
//	  of them point to symbols in the symbol table that must be resolved
//	  by the dynamic linker.
//
// To summarize what we need to do:
//
//	* Find the shared libraries linked by the program
//	* Locate the dynamic symbol and string tables, and the relocation tables.
//	* Find relocation entries that point to symbols named "malloc", "calloc",
//	  etc.
//	* For each of those entries, edit the address of the symbol to point to
//	  our wrappers.

extern void *__wrap_malloc(size_t size);
extern void *__wrap_calloc(size_t nmemb, size_t size);
extern void *__wrap_realloc(void *p, size_t size);
extern void *__wrap_valloc(size_t size);
extern void *__wrap_aligned_alloc(size_t alignment, size_t size);
extern int __wrap_posix_memalign(void **p, size_t alignment, size_t size);

struct alloc_func {
	char *name;
	void *addr;
};

struct alloc_func alloc_funcs[] = {
	{.name="malloc", .addr=&__wrap_malloc},
	{.name="calloc", .addr=&__wrap_calloc},
	{.name="realloc", .addr=&__wrap_realloc},
	{.name="valloc", .addr=&__wrap_valloc},
	{.name="aligned_alloc", .addr=&__wrap_aligned_alloc},
	{.name="posix_memalign", .addr=&__wrap_posix_memalign},
};

static void write_table_entry(ElfW(Addr) addr, void *value) {
	// The memory page containing the relocation table entry might not be
	// writable. Without changing the permissions for the page, the
	// subsequent memcpy will segfault
	long page_size = sysconf(_SC_PAGESIZE);
	void *page = (void *)((addr / page_size) * page_size);
	mprotect(page, page_size, PROT_READ | PROT_WRITE);
	memcpy((void *)(addr), &value, sizeof(void *));
}

static void traverse_rels(ElfW(Rel) *rels, size_t nrels, ElfW(Sym) *syms, char *strings, ElfW(Addr) base) {
	for (size_t i = 0; i < nrels; i++) {
		ElfW(Rel) *rel = &rels[i];
		ElfW(Sym) *sym = &syms[ELF64_R_SYM(rel->r_info)];
		char *name = &strings[sym->st_name];
		ElfW(Addr) table_entry = rel->r_offset + base;
		for (int j = 0; j < sizeof(alloc_funcs) / sizeof(struct alloc_func); j++) {
			struct alloc_func f = alloc_funcs[j];
			if (strcmp(name, f.name) == 0) {
				write_table_entry(table_entry, f.addr);
				break;
			}
		}
	}
}

static void traverse_relas(ElfW(Rela) *relas, size_t nrelas, ElfW(Sym) *syms, char *strings, ElfW(Addr) base) {
	for (size_t i = 0; i < nrelas; i++) {
		ElfW(Rela) *rel = &relas[i];
		ElfW(Sym) *sym = &syms[ELF64_R_SYM(rel->r_info)];
		char *name = &strings[sym->st_name];
		ElfW(Addr) table_entry = rel->r_offset + base;
		for (int j = 0; j < sizeof(alloc_funcs) / sizeof(struct alloc_func); j++) {
			struct alloc_func f = alloc_funcs[j];
			if (strcmp(name, f.name) == 0) {
				write_table_entry(table_entry, f.addr);
				break;
			}
		}
	}
}

// glibc's dynamic linker implementation seems to adjust the d_ptr entries (e.g.
// DT_SYMTAB, DT_STRTAB) in the PT_DYNAMIC by the memory base address before
// they're accessed through dl_iterate_phdr. This is inconsistent with the ELF
// documentation which states that "when interpreting these addresses, the
// actual address should be computed based on the original file value and memory
// base address". See https://sourceware.org/git/?p=glibc.git;a=blob_plain;f=elf/get-dynamic-info.h;hb=glibc-2.35
//
// For reference, musl libc does what's arguably the correct thing and doesn't
// adjust the addresses.

#ifdef __GLIBC__
#define GLIBC_ADJUST(x) (0)
#else
#define GLIBC_ADJUST(x) (x)
#endif

static int callback(struct dl_phdr_info *info, size_t size, void *data) {
	int *count = data;
	if ((*count)++ == 0) {
		// The program itself will be the first object visited by
		// dl_iterate_phdr. Don't edit the relocation table for the
		// program itself since we're already hooking into allocations
		// through the linker using --wrap
		return 0;
	}

	if (strstr(info->dlpi_name, "linux-vdso") || strstr(info->dlpi_name, "/ld-linux")) {
		// We don't want to touch the DSOs provided by the kernel
		return 0;
	}

	for (ElfW(Half) i = 0; i < info->dlpi_phnum; i++) {
		ElfW(Phdr) hdr = info->dlpi_phdr[i];
		if (hdr.p_type != PT_DYNAMIC) {
			continue;
		}

		// We're looking for:
		//      * The symbol table & string table to get the symbol names
		//      * The relocation tables (REL, RELA, and JMPREL) to edit

		char *strings = NULL;
		ElfW(Sym) *symbols = NULL;
		ElfW(Rel) *rel = NULL;
		size_t rel_size = 0;
		ElfW(Rela) *rela = NULL;
		size_t rela_size = 0;
		ElfW(Rela) *jmprel = NULL;
		size_t jmprel_size = 0;

		for (ElfW(Dyn) *dyn = (ElfW(Dyn) *) (hdr.p_vaddr + info->dlpi_addr); dyn->d_tag != 0; dyn++) {
			switch (dyn->d_tag) {
			case DT_SYMTAB:
				symbols = (ElfW(Sym) *) (dyn->d_un.d_ptr + GLIBC_ADJUST(info->dlpi_addr));
				break;
			case DT_STRTAB:
				strings = (char *) (dyn->d_un.d_ptr + GLIBC_ADJUST(info->dlpi_addr));
				break;
			case DT_REL:
				rel = (ElfW(Rel) *) (dyn->d_un.d_ptr + GLIBC_ADJUST(info->dlpi_addr));
				break;
			case DT_RELSZ:
				rel_size = (size_t) dyn->d_un.d_val;
				break;
			case DT_RELA:
				rela = (ElfW(Rela) *) (dyn->d_un.d_ptr + GLIBC_ADJUST(info->dlpi_addr));
				break;
			case DT_RELASZ:
				rela_size = (size_t) dyn->d_un.d_val;
				break;
			case DT_JMPREL:
				jmprel = (ElfW(Rela) *) (dyn->d_un.d_ptr + GLIBC_ADJUST(info->dlpi_addr));
				break;
			case DT_PLTRELSZ:
				jmprel_size = (size_t) dyn->d_un.d_val;
				break;
			}
		}

		if ((symbols != NULL) && (strings != NULL)) {
			if (rel != NULL) {
				traverse_rels(rel, rel_size / sizeof(ElfW(Rel)), symbols, strings, info->dlpi_addr);
			}

			if (rela != NULL) {
				traverse_relas(rela, rela_size / sizeof(ElfW(Rela)), symbols, strings, info->dlpi_addr);
			}

			if (jmprel != NULL) {
				traverse_relas(jmprel, jmprel_size / sizeof(ElfW(Rela)), symbols, strings, info->dlpi_addr);
			}
		}
	}
	return 0;

	// TODO(?): look for C.malloc wrappers in symbol table? Maybe more
	// reliable than parsing symbol table from Go (accounting for offsets
	// due to where stuff ends up in memory, etc?) and more efficient than
	// looking up through dladdr?
}

__attribute__ ((constructor)) static void find_dynamic_mallocs(void) {
	int count = 0;
	dl_iterate_phdr(callback, &count);
}