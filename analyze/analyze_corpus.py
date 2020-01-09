import sys
import os
import simplejson as json
import matplotlib.pyplot as plt
from matplotlib.collections import PatchCollection
from matplotlib.patches import Rectangle
import numpy as np

from plot import plot
from utils import loadDataCached

def __processTest(test):
    ret = []
    fn = 'debug_' + test
    if not os.path.isfile(fn):
        return ret;
    count = 0;
    t_bgn = -1;
    t_max = 0;
    prev_ts = -1;
    corpusCount = 0;
    corpusFPCount = 0;
    f = open(fn)
    for line in f:
        line = line.strip('\n').strip();
        if len(line) == 0:
            continue;
        if line[:3] == "<<<":
            ts = int(line.strip("<<<").strip(">>>"))
            if t_bgn < 0:
                t_bgn = ts;
            ts = ts - t_bgn
            t_max = max(t_max, ts)
        elif line[0] == '-' and 'executeRaw' in line:
            count += 1;
        elif "addInputToCorpus" in line:
            corpusCount += 1;
            if int(line.split(',')[-1]) == 0:
                corpusFPCount += 1;
            tmp = line.split();
            ts = int(tmp[1]) - t_bgn 
            print(int(tmp[1]),t_bgn)
            ret.append((count, ts / 1000000000, corpusCount, corpusFPCount))
    f.close();
    return ret, t_max, count;

def plotCorpus(tests=["KCOV", "RAMINDEX"]):
    datas = {}
    tmax = 0
    cmax = 0
    for test in tests:
        data = {};
        #__data, t, c = __processTest(test);
        __data, t, c = loadDataCached("corpus_%s.cache", test, __processTest)
        tmax = max(tmax, t)
        cmax = max(cmax, c)
        if len(__data) < 1:
            continue;
        data[test] = [(v[0],v[1],v[2]) for v in __data];
        data[test + " FP"] = [(v[0],v[1],v[3]) for v in __data]
        datas[test] = data
    for test in datas:
        datas[test][test].insert(0, (0,0,0))
        datas[test][test + " FP"].insert(0, (0,0,0))
        datas[test][test].append((cmax,tmax,datas[test][test][-1][2]))
        datas[test][test + " FP"].append((cmax,tmax,datas[test][test + " FP"][-1][2]))
        plot(datas[test], 1, 2, xlabel="Time elapsed (s)", ylabel="# of corpus", title="", outfile="corpus_%s.png" % test, xmax=tmax);
