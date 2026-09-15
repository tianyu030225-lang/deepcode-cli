#include "permission.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <termios.h>
#include <unistd.h>

#include "util.h"

dc_native_result dc_prompt_danger_permission(const char *label)
{
    struct termios oldt;
    struct termios raw;
    const char *options[] = {"拒绝", "允许本次"};
    int selected = 0;
    int armed = 0;
    unsigned char ch;

    if (label == NULL) {
        label = "危险操作";
    }

    printf("\033[31m  ○ %s — 需要授权\033[0m\n", label);
    fflush(stdout);

    if (tcgetattr(STDIN_FILENO, &oldt) != 0) {
        return dc_result(0, 0, dc_strdup_safe("canceled"), NULL);
    }
    raw = oldt;
    raw.c_iflag &= ~(IGNBRK | BRKINT | PARMRK | ISTRIP | INLCR | IGNCR | ICRNL | IXON);
    raw.c_lflag &= ~(ECHO | ECHONL | ICANON | ISIG | IEXTEN);
    raw.c_cflag &= ~(CSIZE | PARENB);
    raw.c_cflag |= CS8;
    raw.c_cc[VMIN] = 0;
    raw.c_cc[VTIME] = 1;
    if (tcsetattr(STDIN_FILENO, TCSANOW, &raw) != 0) {
        return dc_result(0, 0, dc_strdup_safe("canceled"), NULL);
    }

    printf("\0337\033[?25l");
    for (;;) {
        int i;
        printf("\0338\033[J");
        printf("\033[38;2;64;156;255m    选择授权\033[0m\n");
        for (i = 0; i < 2; i++) {
            if (i == selected) {
                printf("\033[38;2;64;156;255m    › %s\033[0m\n", options[i]);
            } else {
                printf("      %s\n", options[i]);
            }
        }
        fflush(stdout);

        if (read(STDIN_FILENO, &ch, 1) <= 0) {
            continue;
        }
        if (ch == '\r' || ch == '\n') {
            if (!armed) {
                continue;
            }
            tcsetattr(STDIN_FILENO, TCSANOW, &oldt);
            printf("\0338\033[J\033[?25h");
            if (selected == 1) {
                printf("\033[2m    已授权本次\033[0m\n");
                return dc_result(0, 0, dc_strdup_safe("allowed"), NULL);
            }
            printf("\033[2m    已拒绝\033[0m\n");
            return dc_result(0, 0, dc_strdup_safe("denied"), NULL);
        }
        if (ch == 'y' || ch == 'Y') {
            tcsetattr(STDIN_FILENO, TCSANOW, &oldt);
            printf("\0338\033[J\033[?25h\033[2m    已授权本次\033[0m\n");
            return dc_result(0, 0, dc_strdup_safe("allowed"), NULL);
        }
        if (ch == 'n' || ch == 'N') {
            tcsetattr(STDIN_FILENO, TCSANOW, &oldt);
            printf("\0338\033[J\033[?25h\033[2m    已拒绝\033[0m\n");
            return dc_result(0, 0, dc_strdup_safe("denied"), NULL);
        }
        if (ch == 3 || ch == 27) {
            unsigned char seq[2];
            if (ch == 27 && read(STDIN_FILENO, &seq[0], 1) > 0 && read(STDIN_FILENO, &seq[1], 1) > 0 && seq[0] == '[') {
                if (seq[1] == 'A') {
                    selected = 0;
                    armed = 1;
                    continue;
                }
                if (seq[1] == 'B') {
                    selected = 1;
                    armed = 1;
                    continue;
                }
            }
            tcsetattr(STDIN_FILENO, TCSANOW, &oldt);
            printf("\0338\033[J\033[?25h\033[2m    已取消\033[0m\n");
            return dc_result(0, 0, dc_strdup_safe("canceled"), NULL);
        }
        if (ch == 'k' || ch == 'K') {
            selected = 0;
            armed = 1;
        } else if (ch == 'j' || ch == 'J') {
            selected = 1;
            armed = 1;
        }
    }
}
