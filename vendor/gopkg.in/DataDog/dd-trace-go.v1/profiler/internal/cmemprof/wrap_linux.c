// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

#define _GNU_SOURCE
#include <dlfcn.h>
#include <stdatomic.h>
#include <stdint.h>

#include "profiler_internal.h"

/* The GNU linker supports a --wrap flag that lets wrap allocation calls
 * without the problem of using dlsym in functions called by dlsym */

extern __thread int in_allocation;

void *__real_malloc(size_t size);
void *__wrap_malloc(size_t size) {
	void *ret_addr = __builtin_return_address(0);
	profile_allocation_checked(size, ret_addr);
	in_allocation++;
	void *res = __real_malloc(size);
	in_allocation--;
	return res;
}

void *__real_calloc(size_t nmemb, size_t size);
void *__wrap_calloc(size_t nmemb, size_t size) {
	// If the allocation size would overflow, don't bother profiling, and
	// let the real calloc implementation (possibly) fail.
	if ((size > 0) && (nmemb > (SIZE_MAX/size))) {
		return __real_calloc(nmemb, size);
	}
	profile_allocation(size * nmemb);
	in_allocation++;
	void *res = __real_calloc(nmemb, size);
	in_allocation--;
	return res;
}

void *__real_realloc(void *p, size_t size);
void *__wrap_realloc(void *p, size_t size) {
	profile_allocation(size);
	in_allocation++;
	void *res = __real_realloc(p, size);
	in_allocation--;
	return res;
}

void *__real_valloc(size_t size);
void *__wrap_valloc(size_t size) {
	profile_allocation(size);
	in_allocation++;
	void *res = __real_valloc(size);
	in_allocation--;
	return res;
}

void *__real_aligned_alloc(size_t alignment, size_t size);
void *__wrap_aligned_alloc(size_t alignment, size_t size) {
	profile_allocation(size);
	in_allocation++;
	void *res = __real_aligned_alloc(alignment, size);
	in_allocation--;
	return res;
}

int __real_posix_memalign(void **p, size_t alignment, size_t size);
int __wrap_posix_memalign(void **p, size_t alignment, size_t size) {
	profile_allocation(size);
	in_allocation++;
	int res = __real_posix_memalign(p, alignment, size);
	in_allocation--;
	return res;
}
