#ifndef DEEPCODE_UTIL_H
#define DEEPCODE_UTIL_H

#include <stddef.h>

#include "native.h"

char *dc_strdup_safe(const char *text);
int dc_blank_string(const char *text);
dc_native_result dc_result(int code, int exit_code, char *output, const char *error);
dc_native_result dc_errno_result(const char *prefix);
int dc_append_bytes(char **buf, size_t *len, size_t *cap, const char *data, size_t n);
int dc_read_available_fd(int fd, char **out, size_t *len, size_t *cap, int *eof);
int dc_read_all_fd(int fd, char **out);

#endif
