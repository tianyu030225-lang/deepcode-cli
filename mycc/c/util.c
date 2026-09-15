#include "util.h"

#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

char *dc_strdup_safe(const char *text)
{
    char *copy;
    if (text == NULL) {
        text = "";
    }
    copy = (char *)malloc(strlen(text) + 1);
    if (copy == NULL) {
        return NULL;
    }
    strcpy(copy, text);
    return copy;
}

int dc_blank_string(const char *text)
{
    if (text == NULL) {
        return 1;
    }
    while (*text != '\0') {
        if (*text != ' ' && *text != '\t' && *text != '\n' && *text != '\r') {
            return 0;
        }
        text++;
    }
    return 1;
}

dc_native_result dc_result(int code, int exit_code, char *output, const char *error)
{
    dc_native_result r;
    r.code = code;
    r.exit_code = exit_code;
    r.output = output == NULL ? dc_strdup_safe("") : output;
    r.error = dc_strdup_safe(error == NULL ? "" : error);
    if (r.output == NULL || r.error == NULL) {
        free(r.output);
        free(r.error);
        r.code = -1;
        r.exit_code = -1;
        r.output = dc_strdup_safe("");
        r.error = dc_strdup_safe("out of memory");
    }
    return r;
}

dc_native_result dc_errno_result(const char *prefix)
{
    char msg[512];
    snprintf(msg, sizeof(msg), "%s: %s", prefix, strerror(errno));
    return dc_result(-1, -1, NULL, msg);
}

int dc_append_bytes(char **buf, size_t *len, size_t *cap, const char *data, size_t n)
{
    if (*buf == NULL) {
        *cap = 4096;
        *len = 0;
        *buf = (char *)malloc(*cap + 1);
        if (*buf == NULL) {
            return -1;
        }
    }
    while (*len + n > *cap) {
        char *next;
        *cap *= 2;
        next = (char *)realloc(*buf, *cap + 1);
        if (next == NULL) {
            return -1;
        }
        *buf = next;
    }
    memcpy(*buf + *len, data, n);
    *len += n;
    (*buf)[*len] = '\0';
    return 0;
}

int dc_read_available_fd(int fd, char **out, size_t *len, size_t *cap, int *eof)
{
    char tmp[4096];
    int reads;
    for (reads = 0; reads < 64; reads++) {
        ssize_t n = read(fd, tmp, sizeof(tmp));
        if (n > 0) {
            if (dc_append_bytes(out, len, cap, tmp, (size_t)n) != 0) {
                return -1;
            }
            continue;
        }
        if (n == 0) {
            *eof = 1;
            return 0;
        }
        if (errno == EINTR) {
            continue;
        }
        if (errno == EAGAIN || errno == EWOULDBLOCK) {
            return 0;
        }
        return -1;
    }
    return 0;
}

int dc_read_all_fd(int fd, char **out)
{
    size_t cap = 4096;
    size_t len = 0;
    char *buf = (char *)malloc(cap + 1);
    if (buf == NULL) {
        return -1;
    }
    for (;;) {
        ssize_t n;
        if (len == cap) {
            char *next;
            cap *= 2;
            next = (char *)realloc(buf, cap + 1);
            if (next == NULL) {
                free(buf);
                return -1;
            }
            buf = next;
        }
        n = read(fd, buf + len, cap - len);
        if (n < 0) {
            if (errno == EINTR) {
                continue;
            }
            free(buf);
            return -1;
        }
        if (n == 0) {
            break;
        }
        len += (size_t)n;
    }
    buf[len] = '\0';
    *out = buf;
    return 0;
}

void dc_free_result(dc_native_result *result)
{
    if (result == NULL) {
        return;
    }
    free(result->output);
    free(result->error);
    result->output = NULL;
    result->error = NULL;
}
