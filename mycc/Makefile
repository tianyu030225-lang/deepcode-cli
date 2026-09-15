GO ?= go
CC ?= cc
CFLAGS ?= -std=c11 -D_POSIX_C_SOURCE=200809L -Wall -Wextra -O2 -fPIC
BUILD_DIR ?= ../build
BIN := $(BUILD_DIR)/deepcode
NATIVE_BUILD_DIR := .build
NATIVE_LIB := $(NATIVE_BUILD_DIR)/libdeepcode_native.a
GO_SRC := $(wildcard go/*.go)
C_SRC := c/util.c c/file_ops.c c/exec.c c/permission.c c/path_utils.c c/json_utils.c c/sub_agent.c c/cJSON.c
C_OBJ := $(patsubst c/%.c,$(NATIVE_BUILD_DIR)/%.o,$(C_SRC))
HDR := c/native.h c/util.h c/file_ops.h c/exec.h c/permission.h c/path_utils.h c/json_utils.h c/sub_agent.h c/cJSON.h

.PHONY: all clean test race vet

all: $(BIN)

$(BIN): $(GO_SRC) $(NATIVE_LIB) Makefile
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 $(GO) build -o $(BIN) ./go

$(NATIVE_BUILD_DIR)/%.o: c/%.c $(HDR) Makefile
	mkdir -p $(NATIVE_BUILD_DIR)
	$(CC) $(CFLAGS) -Ic -c $< -o $@

$(NATIVE_LIB): $(C_OBJ)
	ar rcs $@ $^

test: $(NATIVE_LIB)
	CGO_ENABLED=1 $(GO) test ./go

race: $(NATIVE_LIB)
	CGO_ENABLED=1 $(GO) test -race ./go

vet: $(NATIVE_LIB)
	CGO_ENABLED=1 $(GO) vet ./go

clean:
	rm -rf $(NATIVE_BUILD_DIR) $(BIN) deepcode
