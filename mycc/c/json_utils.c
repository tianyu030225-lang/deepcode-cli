#include "json_utils.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "cJSON.h"
#include "util.h"

dc_native_result dc_build_chat_request_json(const char *model, const char *messages_json, const char *tools_json, int thinking)
{
    cJSON *root = cJSON_CreateObject();
    cJSON *messages = NULL;
    cJSON *tools = NULL;
    char *printed = NULL;

    if (root == NULL) {
        return dc_result(-1, -1, NULL, "cJSON object allocation failed");
    }
    cJSON_AddStringToObject(root, "model", model == NULL ? "" : model);
    messages = cJSON_Parse(messages_json == NULL || messages_json[0] == '\0' ? "[]" : messages_json);
    if (!cJSON_IsArray(messages)) {
        cJSON_Delete(root);
        cJSON_Delete(messages);
        return dc_result(-1, -1, NULL, "messages must be a JSON array");
    }
    cJSON_AddItemToObject(root, "messages", messages);
    cJSON_AddBoolToObject(root, "stream", 1);

    if (tools_json != NULL && tools_json[0] != '\0') {
        tools = cJSON_Parse(tools_json);
        if (!cJSON_IsArray(tools)) {
            cJSON_Delete(root);
            cJSON_Delete(tools);
            return dc_result(-1, -1, NULL, "tools must be a JSON array");
        }
        if (cJSON_GetArraySize(tools) > 0) {
            cJSON_AddItemToObject(root, "tools", tools);
        } else {
            cJSON_Delete(tools);
        }
    }

    if (thinking) {
        cJSON *thinking_obj = cJSON_CreateObject();
        cJSON_AddStringToObject(root, "reasoning_effort", "high");
        if (thinking_obj == NULL) {
            cJSON_Delete(root);
            return dc_result(-1, -1, NULL, "thinking object allocation failed");
        }
        cJSON_AddStringToObject(thinking_obj, "type", "enabled");
        cJSON_AddItemToObject(root, "thinking", thinking_obj);
    }

    printed = cJSON_PrintUnformatted(root);
    cJSON_Delete(root);
    if (printed == NULL) {
        return dc_result(-1, -1, NULL, "cJSON print failed");
    }
    return dc_result(0, 0, printed, NULL);
}

dc_native_result dc_parse_sse_chunk_json(const char *data)
{
    cJSON *root = cJSON_Parse(data == NULL ? "" : data);
    cJSON *out = cJSON_CreateObject();
    cJSON *error_obj;
    cJSON *usage;
    cJSON *choices;
    cJSON *choice;
    cJSON *delta;
    cJSON *item;
    char *printed = NULL;

    if (root == NULL) {
        cJSON_Delete(out);
        return dc_result(-1, -1, NULL, "解析流式响应失败");
    }
    if (out == NULL) {
        cJSON_Delete(root);
        return dc_result(-1, -1, NULL, "cJSON object allocation failed");
    }

    error_obj = cJSON_GetObjectItemCaseSensitive(root, "error");
    if (cJSON_IsObject(error_obj)) {
        item = cJSON_GetObjectItemCaseSensitive(error_obj, "message");
        if (cJSON_IsString(item)) {
            cJSON_AddStringToObject(out, "error", item->valuestring);
        }
    }

    usage = cJSON_GetObjectItemCaseSensitive(root, "usage");
    if (cJSON_IsObject(usage)) {
        cJSON_AddItemToObject(out, "usage", cJSON_Duplicate(usage, 1));
    }

    choices = cJSON_GetObjectItemCaseSensitive(root, "choices");
    choice = cJSON_IsArray(choices) ? cJSON_GetArrayItem(choices, 0) : NULL;
    delta = cJSON_IsObject(choice) ? cJSON_GetObjectItemCaseSensitive(choice, "delta") : NULL;
    if (cJSON_IsObject(delta)) {
        item = cJSON_GetObjectItemCaseSensitive(delta, "content");
        if (cJSON_IsString(item)) {
            cJSON_AddStringToObject(out, "content", item->valuestring);
        }
        item = cJSON_GetObjectItemCaseSensitive(delta, "reasoning_content");
        if (cJSON_IsString(item)) {
            cJSON_AddStringToObject(out, "reasoning_content", item->valuestring);
        }
        item = cJSON_GetObjectItemCaseSensitive(delta, "tool_calls");
        if (cJSON_IsArray(item)) {
            cJSON_AddItemToObject(out, "tool_calls", cJSON_Duplicate(item, 1));
        }
    }

    printed = cJSON_PrintUnformatted(out);
    cJSON_Delete(root);
    cJSON_Delete(out);
    if (printed == NULL) {
        return dc_result(-1, -1, NULL, "cJSON print failed");
    }
    return dc_result(0, 0, printed, NULL);
}

dc_native_result dc_parse_tool_args_json(const char *name, const char *raw_args)
{
    cJSON *root = cJSON_Parse(raw_args == NULL || raw_args[0] == '\0' ? "{}" : raw_args);
    cJSON *out = cJSON_CreateObject();
    cJSON *item;
    char *printed = NULL;

    if (root == NULL || !cJSON_IsObject(root)) {
        cJSON_Delete(root);
        cJSON_Delete(out);
        return dc_result(-1, -1, NULL, "工具参数 JSON 解析失败");
    }
    if (out == NULL) {
        cJSON_Delete(root);
        return dc_result(-1, -1, NULL, "cJSON object allocation failed");
    }

    if (strcmp(name, "get_cwd") == 0) {
        printed = cJSON_PrintUnformatted(out);
    } else if (strcmp(name, "read_file") == 0 || strcmp(name, "delete_file") == 0) {
        item = cJSON_GetObjectItemCaseSensitive(root, "path");
        if (!cJSON_IsString(item) || dc_blank_string(item->valuestring)) {
            cJSON_Delete(root);
            cJSON_Delete(out);
            return dc_result(-1, -1, NULL, "工具参数 path 不能为空");
        }
        cJSON_AddStringToObject(out, "path", item->valuestring);
        printed = cJSON_PrintUnformatted(out);
    } else if (strcmp(name, "write_file") == 0) {
        item = cJSON_GetObjectItemCaseSensitive(root, "path");
        if (!cJSON_IsString(item) || dc_blank_string(item->valuestring)) {
            cJSON_Delete(root);
            cJSON_Delete(out);
            return dc_result(-1, -1, NULL, "工具参数 path 不能为空");
        }
        cJSON_AddStringToObject(out, "path", item->valuestring);
        item = cJSON_GetObjectItemCaseSensitive(root, "content");
        if (!cJSON_IsString(item)) {
            cJSON_Delete(root);
            cJSON_Delete(out);
            return dc_result(-1, -1, NULL, "工具参数 content 不能为空");
        }
        cJSON_AddStringToObject(out, "content", item->valuestring);
        printed = cJSON_PrintUnformatted(out);
    } else if (strcmp(name, "execute_bash") == 0) {
        item = cJSON_GetObjectItemCaseSensitive(root, "command");
        if (!cJSON_IsString(item) || dc_blank_string(item->valuestring)) {
            cJSON_Delete(root);
            cJSON_Delete(out);
            return dc_result(-1, -1, NULL, "工具参数 command 不能为空");
        }
        cJSON_AddStringToObject(out, "command", item->valuestring);
        printed = cJSON_PrintUnformatted(out);
    } else {
        printed = cJSON_PrintUnformatted(root);
    }

    cJSON_Delete(root);
    cJSON_Delete(out);
    if (printed == NULL) {
        return dc_result(-1, -1, NULL, "cJSON print failed");
    }
    return dc_result(0, 0, printed, NULL);
}
