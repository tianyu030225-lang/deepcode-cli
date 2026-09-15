#ifndef DEEPCODE_NATIVE_H
#define DEEPCODE_NATIVE_H

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    int code;
    int exit_code;
    char *output;
    char *error;
} dc_native_result;

dc_native_result dc_read_file(const char *path);
dc_native_result dc_write_file(const char *path, const char *content);
dc_native_result dc_delete_file(const char *path);
dc_native_result dc_execute_bash(const char *command);
dc_native_result dc_execute_bash_cancelable(const char *command, int cancel_fd, int new_process_group);
dc_native_result dc_workspace_read_file(const char *path);
dc_native_result dc_workspace_write_file(const char *path, const char *content);
dc_native_result dc_workspace_delete_file(const char *path);
int dc_open_workspace_file(const char *path, int flags);
dc_native_result dc_run_sub_agent(const char *exe, const char *task, const char *config_path, const char *session_path, int danger_next, int very_danger);
dc_native_result dc_run_sub_agent_cancelable(const char *exe, const char *task, const char *config_path, const char *session_path, int danger_next, int very_danger, int cancel_fd);
dc_native_result dc_prompt_danger_permission(const char *label);
dc_native_result dc_workspace_file_path(const char *path);
dc_native_result dc_build_chat_request_json(const char *model, const char *messages_json, const char *tools_json, int thinking);
dc_native_result dc_parse_sse_chunk_json(const char *data);
dc_native_result dc_parse_tool_args_json(const char *name, const char *raw_args);
void dc_free_result(dc_native_result *result);

#ifdef __cplusplus
}
#endif

#endif
