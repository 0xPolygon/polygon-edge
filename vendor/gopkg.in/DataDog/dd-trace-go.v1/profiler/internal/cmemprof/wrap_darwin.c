// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

#include <dlfcn.h>
#include <stddef.h>
#include <stdint.h>

#include <pthread.h>

#include "profiler_internal.h"

void *(*real_malloc)(size_t);
void *(*real_calloc)(size_t, size_t);
void *(*real_realloc)(void *, size_t);
void *(*real_valloc)(size_t);
void *(*real_aligned_alloc)(size_t, size_t);
int (*real_posix_memalign)(void **, size_t, size_t);

pthread_once_t alloc_funcs_init_once;
static void alloc_funcs_init(void) {
	void *f = NULL;
	f = dlsym(RTLD_NEXT, "malloc");
	if (f != NULL) {
		real_malloc = f;
	}
	f = dlsym(RTLD_NEXT, "calloc");
	if (f != NULL) {
		real_calloc = f;
	}
	f = dlsym(RTLD_NEXT, "realloc");
	if (f != NULL) {
		real_realloc = f;
	}
	f = dlsym(RTLD_NEXT, "valloc");
	if (f != NULL) {
		real_valloc = f;
	}
	f = dlsym(RTLD_NEXT, "aligned_alloc");
	if (f != NULL) {
		real_aligned_alloc = f;
	}
	f = dlsym(RTLD_NEXT, "posix_memalign");
	if (f != NULL) {
		real_posix_memalign = f;
	}
}


void *malloc(size_t size) {
	pthread_once(&alloc_funcs_init_once, alloc_funcs_init);
	void *ret_addr = __builtin_return_address(0);
	profile_allocation_checked(size, ret_addr);
	return real_malloc(size);
}

void *calloc(size_t nmemb, size_t size) {
	pthread_once(&alloc_funcs_init_once, alloc_funcs_init);
	// If the allocation size would overflow, don't bother profiling, and
	// let the real calloc implementation (possibly) fail.
	if ((size > 0) && (nmemb > (SIZE_MAX/size))) {
		return real_calloc(nmemb, size);
	}
	profile_allocation(size * nmemb);
	return real_calloc(nmemb, size);
}

void *realloc(void *p, size_t size) {
	pthread_once(&alloc_funcs_init_once, alloc_funcs_init);
	profile_allocation(size);
	return real_realloc(p, size);
}

void *valloc(size_t size) {
	pthread_once(&alloc_funcs_init_once, alloc_funcs_init);
	profile_allocation(size);
	return real_valloc(size);
}

void *aligned_alloc(size_t alignment, size_t size) {
	pthread_once(&alloc_funcs_init_once, alloc_funcs_init);
	profile_allocation(size);
	return real_aligned_alloc(alignment, size);
}

int posix_memalign(void **p, size_t alignment, size_t size) {
	pthread_once(&alloc_funcs_init_once, alloc_funcs_init);
	profile_allocation(size);
	return real_posix_memalign(p, alignment, size);
}
