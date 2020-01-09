#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

struct timespec tsc_timespec;

float getTime() {
    clock_gettime(CLOCK_MONOTONIC, &tsc_timespec);
    // syscall(__NR_clock_gettime, CLOCK_MONOTONIC, &tsc_timespec);
    return (float) tsc_timespec.tv_sec + (float) tsc_timespec.tv_nsec / 1000000000.0;
}

int main(int argc, char *argv[]) {
    if (argc < 3) {
        printf("Usage: filter_log input output\n");
        return 1;
    }
    float ts_bgn = getTime();
    printf("Filtering log from %s to %s\n", argv[1], argv[2]); 
    FILE *fin = fopen(argv[1], "r");
    FILE *fout = fopen(argv[2], "w");
#define BUF_SIZE 4096
    unsigned char c;
    unsigned char prev;
    int newline = 1;
    int output = 0;
    unsigned long lines = 1;
 
    while (!feof(fin)){                        // while not end of file
        c=fgetc(fin);                         // get a character/byte from the file
        if (newline == 1) {
            switch(c) {
            case '#':
            case '-':
            case '=':
            case '+':
                output = 1;
                newline = 2;
                // fputc(c, fout);
                prev = c;
                break;
            case '>':
            case '<':
                output = 1;
                newline = 0;
                fputc(c, fout);
                break;
            default:
                output = 0;
                newline = 0;
            }
        } else if (newline == 2) {
            switch(c) {
            case ' ':
                fputc(prev, fout);
                fputc(c, fout);
                newline = 0;
                break;
            default:
                output = 0;
                newline = 0;
            }
        } else if (c == '\n') {
            if (output)
                fputc(c, fout);
            newline = 1;
            lines ++;
            if (lines % 100000 == 0)
                printf("Filtering log from %s to %s. %lu lines\n", argv[1], argv[2], lines);
        } else if (output) {
            fputc(c & 0x7f, fout);
        }
    }
    fclose(fin);                               // close the file
    fclose(fout);
    float ts_end = getTime();
    printf("Finished filtering log from %s to %s. Takes %f seconds.\n", argv[1], argv[2], ts_end - ts_bgn);
    return 0;
}
