#include "file_ops.h"

#include <errno.h>
#include <fcntl.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

#include "util.h"
#include "path_utils.h"

static dc_native_result dc_read_file_impl(const char *path, int workspace)
{
    int fd;
    char *content = NULL;
    if (path == NULL || path[0] == '\0') {
        return dc_result(-1, -1, NULL, "empty path");
    }
    fd = workspace ? dc_open_workspace_file(path, O_RDONLY) : open(path, O_RDONLY);
    if (fd < 0) {
        return dc_errno_result("open");
    }
    if (dc_read_all_fd(fd, &content) != 0) {
        close(fd);
        return dc_errno_result("read");
    }
    close(fd);
    return dc_result(0, 0, content, NULL);
}

static dc_native_result dc_write_file_impl(const char *path, const char *content, int workspace)
{
    int fd;
    size_t len;
    size_t off = 0;
    if (path == NULL || path[0] == '\0') {
        return dc_result(-1, -1, NULL, "empty path");
    }
    if (content == NULL) {
        content = "";
    }
    fd = workspace ? dc_open_workspace_file(path, O_WRONLY | O_CREAT | O_TRUNC)
                   : open(path, O_WRONLY | O_CREAT | O_TRUNC, 0644);
    if (fd < 0) {
        return dc_errno_result("open");
    }
    len = strlen(content);
    while (off < len) {
        ssize_t n = write(fd, content + off, len - off);
        if (n < 0) {
            if (errno == EINTR) {
                continue;
            }
            close(fd);
            return dc_errno_result("write");
        }
        off += (size_t)n;
    }
    if (close(fd) != 0) {
        return dc_errno_result("close");
    }
    return dc_result(0, 0, dc_strdup_safe("ok"), NULL);
}

dc_native_result dc_delete_file(const char *path)
{
    if (path == NULL || path[0] == '\0') {
        return dc_result(-1, -1, NULL, "empty path");
    }
    if (unlink(path) != 0) {
        return dc_errno_result("unlink");
    }
    return dc_result(0, 0, dc_strdup_safe("ok"), NULL);
}

dc_native_result dc_read_file(const char *path)
{
    return dc_read_file_impl(path, 0);
}

dc_native_result dc_write_file(const char *path, const char *content)
{
    return dc_write_file_impl(path, content, 0);
}

dc_native_result dc_workspace_read_file(const char *path)
{
    return dc_read_file_impl(path, 1);
}

dc_native_result dc_workspace_write_file(const char *path, const char *content)
{
    return dc_write_file_impl(path, content, 1);
}

dc_native_result dc_workspace_delete_file(const char *path)
{
    char *name;
    struct stat info;
    int parent = dc_open_workspace_parent(path, &name, 0);
    int code;
    int saved_errno;
    if (parent < 0) return dc_errno_result("workspace path");
    code = fstatat(parent, name, &info, AT_SYMLINK_NOFOLLOW);
    if (code == 0 && !S_ISREG(info.st_mode) && !S_ISLNK(info.st_mode)) {
        errno = EINVAL;
        code = -1;
    }
    if (code == 0) code = unlinkat(parent, name, 0);
    saved_errno = errno;
    close(parent);
    free(name);
    errno = saved_errno;
    if (code != 0) return dc_errno_result("unlink");
    return dc_result(0, 0, dc_strdup_safe("ok"), NULL);
}
