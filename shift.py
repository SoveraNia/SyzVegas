#!/bin/python3
import sys
import os
import glob
import shutil

shift = int(sys.argv[1])

flist = glob.glob("*KCOV*")
move_list = []
for fn in flist:
    if not "KERNEL_" in fn:
        continue
    cnt = (fn.split("KERNEL_")[1].replace('_', '.').replace('-', '.').split('.')[0])
    cnt_new = int(cnt) + shift
    fn_new = fn.replace("KERNEL_%s" % cnt, "KERNEL_%s" % str(cnt_new).zfill(3))
    print(fn, fn_new)
    if os.path.exists(fn_new):
        print("Error: %s exists" % fn_new)
        exit(1)
    move_list.append((fn, fn_new))

g = input("Confirm move (Y/N)") 
print(g)
if g == "Y":
    for fn, fn_new in move_list:
        print(fn, fn_new)
        shutil.move(fn, fn_new)
