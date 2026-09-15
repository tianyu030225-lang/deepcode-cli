#include "sub_agent.h"

#include <stdlib.h>
#include <string.h>

#include "exec.h"

dc_native_result dc_run_sub_agent(const char *exe, const char *task, const char *config_path, const char *session_path, int danger_next, int very_danger)
{
    char *argv[11];
    int i = 0;
    argv[i++] = (char *)(exe == NULL ? "deepcode" : exe);
    argv[i++] = "--sub-agent";
    argv[i++] = (char *)(task == NULL ? "" : task);
    argv[i++] = "--config";
    argv[i++] = (char *)(config_path == NULL ? "" : config_path);
    argv[i++] = "--session";
    argv[i++] = (char *)(session_path == NULL ? "" : session_path);
    if (danger_next) {
        argv[i++] = "--danger-next";
    }
    if (very_danger) {
        argv[i++] = "--very-danger";
    }
    argv[i] = NULL;
    return dc_spawn_capture(argv);
}

dc_native_result dc_run_sub_agent_cancelable(const char *exe, const char *task, const char *config_path, const char *session_path, int danger_next, int very_danger, int cancel_fd)
{
    char *argv[11];
    int i = 0;
    argv[i++] = (char *)(exe == NULL ? "deepcode" : exe);
    argv[i++] = "--sub-agent";
    argv[i++] = (char *)(task == NULL ? "" : task);
    argv[i++] = "--config";
    argv[i++] = (char *)(config_path == NULL ? "" : config_path);
    argv[i++] = "--session";
    argv[i++] = (char *)(session_path == NULL ? "" : session_path);
    if (danger_next) {
        argv[i++] = "--danger-next";
    }
    if (very_danger) {
        argv[i++] = "--very-danger";
    }
    argv[i] = NULL;
    return dc_spawn_capture_cancelable(argv, cancel_fd);
}
