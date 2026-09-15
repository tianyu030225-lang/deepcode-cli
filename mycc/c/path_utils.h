#ifndef DEEPCODE_PATH_UTILS_H
#define DEEPCODE_PATH_UTILS_H

#include "native.h"

int dc_open_workspace_parent(const char *path, char **name, int resolve_leaf);
int dc_open_workspace_file(const char *path, int flags);

#endif
