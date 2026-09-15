//go:build ignore

#include <stdio.h>

#define BLUE "\033[38;2;64;156;255m"
#define RESET "\033[0m"

int main(void)
{
    printf(BLUE);
    printf("                   ▐▛███▜▌\n");
    printf("                  ▝▜█████▛▘\n");
    printf("                    ▘▘ ▝▝\n");
    printf(RESET);

    return 0;
}
