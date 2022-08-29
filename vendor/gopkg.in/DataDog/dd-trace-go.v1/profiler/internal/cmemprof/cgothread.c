#include <stdatomic.h>

// in_cgo_start is 1 or 0 depending on whether not the current thread is in
// x_cgo_thread_start. This function is called by cgo programs to create a new
// OS thread, and runs on a Go "m" without a corresponding "curg". As such, it's
// unsafe to call from C into Go in that situation (say, in a wraper for malloc,
// which x_cgo_thread_start calls.)
__thread atomic_int in_cgo_start;

void (*real_cgo_thread_start)(void *);

void cmemprof_set_cgo_thread_start(void (*f)(void *)) {
        real_cgo_thread_start = f;
}

void cmemprof_cgo_thread_start(void *p) {
        atomic_store(&in_cgo_start, 1);
        real_cgo_thread_start(p);
        atomic_store(&in_cgo_start, 0);
}