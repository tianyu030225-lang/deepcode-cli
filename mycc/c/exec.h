#ifndef DEEPCODE_EXEC_H
#define DEEPCODE_EXEC_H

#include "native.h"

dc_native_result dc_spawn_capture_cancelable(char *const argv[], int cancel_fd);
dc_native_result dc_spawn_capture(char *const argv[]);

#endif
