package main

/*
#include <features.h>
#ifdef __GLIBC__
#include <malloc.h>
#else
void malloc_trim(size_t pad){}
#endif
*/
import "C"

func Free() {
	//debug.FreeOSMemory()
	C.malloc_trim(0)
}
