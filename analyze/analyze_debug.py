import sys
import os
import simplejson as json

def __processDebug(fn):
    syscalls_count = [];
    syscalls_list = [];
    status = "NONE";
    exec_idx = 0;
    syscalls_func = set();
    syscalls_full = set();
    f = open(fn);
    for line in f:
        line = line.strip('\n').strip();
        if len(line) == 0:
            continue;
        if "====BEGIN====" in line:
            status = "DATA";
            syscalls_list.append([]);
        elif "====END====" in line:
            status = "NONE";
            syscalls_count.append((exec_idx, len(syscalls_func), len(syscalls_full)));
            exec_idx += 1;
        elif status == "DATA":
            syscalls_full.add(line);
            func = line.split('(')[0].split()[-1];
            if not func in syscalls_func:
                syscalls_list[exec_idx].append(func);
                syscalls_func.add(func);
    f.close();
    return syscalls_count, syscalls_list;

def __outputSyscalls(test, slist):
    fn = 'syscalls_' + test + '.txt';
    f = open(fn, 'w+');
    for i in range(len(slist)):
        for func in slist[i]:
            f.write("%d\t%s\n" % (i, func));
    f.close();

def parseDebug(test):
    if os.path.isfile('syscalls_' + test):
        cache_file = 'syscalls_' + test + '.cache';
        if os.path.isfile(cache_file):
            try:
                f = open(cache_file);
                data = json.load(f);
                f.close();
                print("Successfully load from database %s" % cache_file)
                # __outputSyscalls(test, data[1]);
                return data[0];
            except:
                pass;
        cache_file = 'syscalls_' + test + '.cache';
        scount, slist = __processDebug('syscalls_' + test);
        # __outputSyscalls(test, slist);
        f = open(cache_file, 'w+');
        json.dump([scount, slist], f);
        f.close();
        return scount; 
    return [];
