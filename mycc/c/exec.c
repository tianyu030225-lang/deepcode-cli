#define _GNU_SOURCE
#include "exec.h"

#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>

#include "util.h"

static long long dc_monotonic_ms(void)
{
    struct timespec now;
    clock_gettime(CLOCK_MONOTONIC, &now);
    return (long long)now.tv_sec * 1000 + now.tv_nsec / 1000000;
}

static dc_native_result dc_spawn_capture_mode(char *const argv[], int cancel_fd, int new_process_group)
{
    int pipefd[2];
    pid_t pid;
    char *output = NULL;
    size_t output_len = 0;
    size_t output_cap = 0;
    int status = 0;
    int exit_code = -1;
    int out_eof = 0;
    int canceled = 0;
    int exited = 0;
    int flags;
    int saved_errno = 0;
    const char *failure = NULL;
    long long drain_started = 0;

    /* Atomic CLOEXEC prevents concurrent agents from inheriting each other's
     * capture pipes and keeping unrelated captures open. */
    if (pipe2(pipefd, O_CLOEXEC) != 0) return dc_errno_result("pipe");
    pid = fork();
    if (pid < 0) {
        saved_errno = errno;
        close(pipefd[0]);
        close(pipefd[1]);
        errno = saved_errno;
        return dc_errno_result("fork");
    }
    if (pid == 0) {
        if (new_process_group) setpgid(0, 0);
        close(pipefd[0]);
        if (dup2(pipefd[1], STDOUT_FILENO) < 0 || dup2(pipefd[1], STDERR_FILENO) < 0) _exit(127);
        close(pipefd[1]);
        execvp(argv[0], argv);
        _exit(127);
    }

    if (new_process_group) setpgid(pid, pid);
    close(pipefd[1]);
    flags = fcntl(pipefd[0], F_GETFL, 0);
    if (flags < 0 || fcntl(pipefd[0], F_SETFL, flags | O_NONBLOCK) < 0) {
        failure = "fcntl";
        saved_errno = errno;
        goto cleanup;
    }

    for (;;) {
        struct pollfd fds[2] = {
            {out_eof ? -1 : pipefd[0], POLLIN, 0},
            {cancel_fd, POLLIN, 0}
        };
        int ready = poll(fds, 2, 100);
        if (ready < 0 && errno != EINTR) {
            failure = "poll";
            saved_errno = errno;
            break;
        }
        if (ready >= 0 && cancel_fd >= 0 && fds[1].revents != 0) {
            canceled = 1;
            break;
        }
        if (!out_eof && dc_read_available_fd(pipefd[0], &output, &output_len, &output_cap, &out_eof) != 0) {
            failure = "read pipe";
            saved_errno = errno;
            break;
        }
        if (!exited) {
            siginfo_t child = {0};
            /* Keep the exited leader unreaped until group cleanup so its PID
             * cannot be reused by an unrelated process group. */
            if (waitid(P_PID, pid, &child, WEXITED | WNOHANG | WNOWAIT) != 0) {
                if (errno != EINTR) {
                    failure = "waitid";
                    saved_errno = errno;
                    break;
                }
            } else if (child.si_pid == pid) {
                exited = 1;
                drain_started = dc_monotonic_ms();
            }
        }
        if (exited && out_eof) break;
        if (exited && dc_monotonic_ms() - drain_started >= 1000) {
            failure = "output pipe remained open after process exit";
            break;
        }
    }

cleanup:
    /* Only signal the group created for this invocation. Shells inside a
     * sub-agent inherit its group; the owning parent tears that group down. */
    if (new_process_group) kill(-pid, SIGKILL);
    if (!exited) kill(pid, SIGKILL);
    while (waitpid(pid, &status, 0) < 0 && errno == EINTR) {}
    if (canceled && flags >= 0) {
        (void)dc_read_available_fd(pipefd[0], &output, &output_len, &output_cap, &out_eof);
    }
    close(pipefd[0]);
    if (output == NULL) output = dc_strdup_safe("");
    if (output == NULL) return dc_result(-1, -1, NULL, "out of memory");
    if (canceled) return dc_result(-1, 130, output, "canceled");
    if (failure != NULL) {
        char error[512];
        if (saved_errno != 0) snprintf(error, sizeof(error), "%s: %s", failure, strerror(saved_errno));
        else snprintf(error, sizeof(error), "%s", failure);
        return dc_result(-1, -1, output, error);
    }
    if (WIFEXITED(status)) exit_code = WEXITSTATUS(status);
    else if (WIFSIGNALED(status)) exit_code = 128 + WTERMSIG(status);
    return dc_result(exit_code == 0 ? 0 : -1, exit_code, output, exit_code == 0 ? NULL : "process failed");
}

dc_native_result dc_spawn_capture_cancelable(char *const argv[], int cancel_fd)
{
    return dc_spawn_capture_mode(argv, cancel_fd, 1);
}

dc_native_result dc_spawn_capture(char *const argv[])
{
    return dc_spawn_capture_cancelable(argv, -1);
}

dc_native_result dc_execute_bash_cancelable(const char *command, int cancel_fd, int new_process_group)
{
    char *const argv[] = {"bash", "-c", (char *)(command == NULL ? "" : command), NULL};
    return dc_spawn_capture_mode(argv, cancel_fd, new_process_group);
}

dc_native_result dc_execute_bash(const char *command)
{
    return dc_execute_bash_cancelable(command, -1, 1);
}
