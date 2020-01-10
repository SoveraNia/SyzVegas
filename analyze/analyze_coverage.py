import sys
import os
import traceback
import copy
import simplejson as json
import matplotlib.pyplot as plt
from matplotlib.collections import PatchCollection
from matplotlib.patches import Rectangle
import numpy as np

from utils import loadDataCached, getTestParams, averageData, cliffsDelta
from plot import plot

def __hash(a):
    a = (a ^ 61) ^ (a >> 16);
    a = a + (a << 3);
    a = a ^ (a >> 4);
    a = a * 0x27d4eb2d;
    a = a ^ (a >> 15);
    return a;

def __processTestAltAlt(test):
    ret = []
    fn = 'result_' + test
    if not os.path.isfile(fn):
        return ret;
    f = open(fn);
    executeCount = 0; 
    syscallCount = 0
    coverage = set()
    coverageCorpus = set()
    idx = 0;
    ts_cur = 0;
    ts_bgn = 0;

    cur_status = {
        "Time_Elapsed": 0,
        "Syscall_Count": 0,
        "Program_Count": 0,
        "Corpus_Coverage": 0,
        "Total_Coverage": 0
    }
    
    for line in f:
        line = line.strip('\n').strip();
        if len(line) == 0:
            continue;
        if line[:2] == '- ' and "executeRaw" in line:
            tmp = line.split();
            try:
                n_calls = int(tmp[-1])
            except:
                n_calls = 1
            # ret.append((executeCount, syscallCount, (ts_cur - ts_bgn) / 1000000000.0 / 60.0, len(coverage), len(coverageHashed), corpusSig))
            cur_status["Time_Elapsed"] = (ts_cur - ts_bgn) / 1000000000.0
            cur_status["Total_Coverage"] = len(coverage);
            cur_status["Corpus_Coverage"] = len(coverageCorpus); 
            ret.append(copy.deepcopy(cur_status))
            cur_status["Program_Count"] += 1;
            cur_status["Syscall_Count"] += n_calls;
            syscallCount += n_calls
        if line[:3] == '<<<' and line[-3:] == '>>>':
            line = line.strip('<<<').strip('>>>')
            ts_cur = int(line)
            if ts_bgn == 0:
                ts_bgn = ts_cur
            if ((ts_cur - ts_bgn) / 1000000000.0) % 600.0 < 5:
                print((ts_cur - ts_bgn) / 1000000000.0)
        elif line[0] == '=':
            tmp = line.split();
            try:
                pc = int(tmp[1], 16)
            except:
                continue
            coverage.add(pc);
            '''
            if (pc & 0xffff000000000000) == 0xffff000000000000:
                coverage.add(pc);
                pc = pc & 0xffffffff;
                sig = pc ^ prev_pc;
                coverageHashed.add(sig);
                prev_pc = __hash(pc);
            elif (pc & 0xffffffff00000000) == 0:
                coverage.add(pc);
                coverageHashed.add(pc);
            '''
        elif line[0] == '+':
            tmp = line.split();
            try:
                pc = int(tmp[1], 16)
            except:
                continue
            coverageCorpus.add(pc);
        #elif "# addInputToCorpus" in line:
        #    tmp = line.split(":")[1].split(".")[0].split(",")[1]
        #    corpusSig = int(tmp)
    f.close();
    return ret;

def plotCoverage(tests=["RAMINDEX", "KCOV"]):
    modules = {}
    # Determine whether we should split by module
    splitByModule = False
    for test in tests:
        print(test)
        name, module, run = getTestParams(test)
        if not module in modules:
            modules[module] = []
        modules[module].append(test)
    for module in modules:
        data = {}
        for test in modules[module]:
          try:
            __data = loadDataCached('coverage_%s.cache', test, __processTestAltAlt);
            print(test, len(__data), __data[-1] if len(__data) > 0 else -1)
            name, module, run = getTestParams(test)
            print(name, module, run)
            #plot({test: __data}, "Time_Elapsed", "Total_Coverage", xlabel="Time elapsed (min)", ylabel="Coverage", title="Coverage", outfile="coverage_%s_time.png" % test);
            
            if not name in data:
                data[name] = []
            data[name].append(__data);
          except:
            traceback.print_exc();
            pass;
        # Cliff's Delta
        for name0 in data:
            for name1 in data:
                if name0 <= name1:
                    continue
                tmp = cliffsDelta(data[name0], data[name1], key="Time_Elapsed", value="Total_Coverage", bin_size=10)
                plot({"Cliff's Delta": tmp}, 0, 1, xlabel="Time elapsed (min)", ylabel="Cliff's Delta", outfile="coverage_cd_%s-VS-%s_time.png" % (name0, name1));
        # Average / median result
        tmp = {}
        for name in data:
            tmp[name] = averageData(data[name], key="Time_Elapsed", value="Total_Coverage", bin_size=1)
        plot(tmp, 0, 1, xlabel="Time elapsed (hr)", ylabel="Coverage (# edges)", outfile="coverage_%s_time.png" % module, xunit=3600.0);
        tmp = {}
        for name in data:
            tmp[name] = averageData(data[name], key="Time_Elapsed", value="Total_Coverage", bin_size=1, median=False)
        plot(tmp, 0, 1, xlabel="Time elapsed (hr)", ylabel="Coverage (# edges)", outfile="coverage_%s_time_mean.png" % module, xunit=3600.0);
        tmp = {}
        for name in data:
            tmp[name] = averageData(data[name], key="Syscall_Count", value="Total_Coverage")
        plot(tmp, 0, 1, xlabel="Syscalls executed", ylabel="Coverage", title="Coverage", outfile="coverage_%s_call.png" % module);
        tmp = {}
        for name in data:
            tmp[name] = averageData(data[name], key="Program_Count", value="Total_Coverage")
        plot(tmp, 0, 1, xlabel="Programs executed", ylabel="Coverage", title="Coverage", outfile="coverage_%s_prog.png" % module);
        tmp = {}
        for name in data:
            tmp[name] = averageData(data[name], key="Time_Elapsed", value="Corpus_Coverage", bin_size=1)
        plot(tmp, 0, 1, xlabel="Time elapsed (min)", ylabel="Corpus Signal", title="Coverage", outfile="corpusSig_%s_time.png" % module);

    # plot(data, 1, 3, xlabel="Time elapsed (min)", ylabel="Coverage", title="Hashed Coverage", outfile="coverage_hashed.png");

