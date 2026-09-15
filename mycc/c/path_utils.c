#define _XOPEN_SOURCE 700
#include "path_utils.h"

#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

#include "util.h"

#ifndef PATH_MAX
#define PATH_MAX 4096
#endif

static char *dc_abs_path(const char *path)
{
    char cwd[PATH_MAX];
    size_t cwd_len;
    size_t path_len;
    char *out;

    if (path == NULL || path[0] == '\0') {
        return NULL;
    }
    if (path[0] == '/') {
        return dc_strdup_safe(path);
    }
    if (getcwd(cwd, sizeof(cwd)) == NULL) {
        return NULL;
    }
    cwd_len = strlen(cwd);
    path_len = strlen(path);
    out = (char *)malloc(cwd_len + 1 + path_len + 1);
    if (out == NULL) {
        return NULL;
    }
    memcpy(out, cwd, cwd_len);
    out[cwd_len] = '/';
    memcpy(out + cwd_len + 1, path, path_len + 1);
    return out;
}

static char *dc_parent_path(const char *path)
{
    char *copy = dc_strdup_safe(path);
    char *slash;
    if (copy == NULL) {
        return NULL;
    }
    slash = strrchr(copy, '/');
    if (slash == NULL) {
        strcpy(copy, ".");
        return copy;
    }
    if (slash == copy) {
        copy[1] = '\0';
        return copy;
    }
    *slash = '\0';
    return copy;
}

static char *dc_real_existing_or_ancestor(const char *abs_path)
{
    char resolved[PATH_MAX];
    char *ancestor;
    struct stat info;

    if (realpath(abs_path, resolved) != NULL) {
        return dc_strdup_safe(resolved);
    }
    if (lstat(abs_path, &info) == 0 && S_ISLNK(info.st_mode)) {
        errno = ELOOP;
        return NULL;
    }
    ancestor = dc_parent_path(abs_path);
    while (ancestor != NULL) {
        char *next;
        if (realpath(ancestor, resolved) != NULL) {
            free(ancestor);
            return dc_strdup_safe(resolved);
        }
        if (lstat(ancestor, &info) == 0 && S_ISLNK(info.st_mode)) {
            free(ancestor);
            errno = ELOOP;
            return NULL;
        }
        next = dc_parent_path(ancestor);
        if (next == NULL || strcmp(next, ancestor) == 0) {
            free(next);
            free(ancestor);
            return NULL;
        }
        free(ancestor);
        ancestor = next;
    }
    return NULL;
}

static int dc_path_is_within(const char *root, const char *candidate)
{
    size_t root_len;
    if (root == NULL || candidate == NULL) {
        return 0;
    }
    if (strcmp(root, "/") == 0) {
        return candidate[0] == '/';
    }
    if (strcmp(root, candidate) == 0) {
        return 1;
    }
    root_len = strlen(root);
    return strncmp(root, candidate, root_len) == 0 && candidate[root_len] == '/';
}

dc_native_result dc_workspace_file_path(const char *path)
{
    char cwd[PATH_MAX];
    char real_cwd[PATH_MAX];
    char *abs_path;
    char *real_check;
    char msg[PATH_MAX + 128];

    if (dc_blank_string(path)) {
        return dc_result(-1, -1, NULL, "工具参数 path 不能为空");
    }
    if (getcwd(cwd, sizeof(cwd)) == NULL) {
        return dc_errno_result("获取当前工作目录失败");
    }
    if (realpath(cwd, real_cwd) == NULL) {
        return dc_errno_result("解析当前工作目录失败");
    }
    abs_path = dc_abs_path(path);
    if (abs_path == NULL) {
        return dc_errno_result("解析工具路径失败");
    }
    real_check = dc_real_existing_or_ancestor(abs_path);
    if (real_check == NULL) {
        free(abs_path);
        return dc_result(-1, -1, NULL, "解析工具路径失败: 找不到有效父目录");
    }
    if (!dc_path_is_within(real_cwd, real_check)) {
        snprintf(msg, sizeof(msg), "工具路径超出当前工作目录: %s", path);
        free(abs_path);
        free(real_check);
        return dc_result(-1, -1, NULL, msg);
    }
    free(real_check);
    return dc_result(0, 0, abs_path, NULL);
}

/* Re-resolve at execution, then walk pinned directory descriptors without
 * following replacement symlinks. Existing links within the workspace remain
 * usable by resolving them again immediately before the descriptor walk. */
int dc_open_workspace_parent(const char *path, char **name, int resolve_leaf)
{
    char cwd[PATH_MAX];
    char resolved[PATH_MAX];
    dc_native_result checked = dc_workspace_file_path(path);
    char *cursor;
    int dirfd = -1;
    int saved_errno = EACCES;
    *name = NULL;
    if (checked.code != 0 || getcwd(cwd, sizeof(cwd)) == NULL) goto done;
    if (resolve_leaf && realpath(checked.output, resolved) != NULL) {
        free(checked.output);
        checked.output = dc_strdup_safe(resolved);
    } else {
        char *parent = dc_parent_path(checked.output);
        char *canonical;
        const char *leaf = strrchr(checked.output, '/') + 1;
        if (parent == NULL || realpath(parent, resolved) == NULL) {
            saved_errno = errno;
            free(parent);
            goto done;
        }
        free(parent);
        canonical = malloc(strlen(resolved) + strlen(leaf) + 2);
        if (canonical == NULL) { saved_errno = ENOMEM; goto done; }
        sprintf(canonical, "%s/%s", resolved, leaf);
        free(checked.output);
        checked.output = canonical;
    }
    if (checked.output == NULL || !dc_path_is_within(cwd, checked.output)) goto done;
    cursor = checked.output + strlen(cwd);
    while (*cursor == '/') cursor++;
    dirfd = open(".", O_RDONLY | O_DIRECTORY | O_CLOEXEC);
    if (dirfd < 0) { saved_errno = errno; goto done; }
    for (;;) {
        char *slash = strchr(cursor, '/');
        if (slash != NULL) *slash = '\0';
        if (strcmp(cursor, "..") == 0 || cursor[0] == '\0') {
            saved_errno = EACCES;
            goto failed;
        }
        if (slash == NULL) {
            if (strcmp(cursor, ".") == 0) { saved_errno = EISDIR; goto failed; }
            *name = dc_strdup_safe(cursor);
            if (*name == NULL) { saved_errno = ENOMEM; goto failed; }
            break;
        }
        if (strcmp(cursor, ".") != 0) {
            int next = openat(dirfd, cursor, O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_CLOEXEC);
            if (next < 0) { saved_errno = errno; goto failed; }
            close(dirfd);
            dirfd = next;
        }
        cursor = slash + 1;
        while (*cursor == '/') cursor++;
    }
    goto done;
failed:
    close(dirfd);
    dirfd = -1;
done:
    dc_free_result(&checked);
    errno = saved_errno;
    return dirfd;
}

int dc_open_workspace_file(const char *path, int flags)
{
    char *name;
    struct stat info;
    int parent = dc_open_workspace_parent(path, &name, 1);
    int fd;
    int saved_errno;
    if (parent < 0) return -1;
    if (fstatat(parent, name, &info, AT_SYMLINK_NOFOLLOW) == 0 && !S_ISREG(info.st_mode)) {
        close(parent);
        free(name);
        errno = EINVAL;
        return -1;
    }
    /* Do not truncate until the opened descriptor has passed the type check. */
    fd = openat(parent, name, (flags & ~O_TRUNC) | O_NOFOLLOW | O_CLOEXEC | O_NONBLOCK, 0644);
    saved_errno = errno;
    close(parent);
    free(name);
    if (fd >= 0) {
        if (fstat(fd, &info) != 0 || !S_ISREG(info.st_mode)) {
            saved_errno = EINVAL;
            close(fd);
            fd = -1;
        } else if ((flags & O_TRUNC) && ftruncate(fd, 0) != 0) {
            saved_errno = errno;
            close(fd);
            fd = -1;
        }
    }
    errno = saved_errno;
    return fd;
}
