import sys
import os
import traceback
import simplejson as json
import matplotlib.pyplot as plt
from matplotlib.collections import PatchCollection
from matplotlib.patches import Rectangle
import numpy as np

def loadDataCached(cache_fn_fmt, test, func):
    cache_fn = cache_fn_fmt % test
    if os.path.isfile(cache_fn):
       try:
           f = open(cache_fn)
           data = json.load(f)
           f.close();
           return data;
       except:
           pass
    data = func(test)
    f = open(cache_fn, "w+")
    json.dump(data, f)
    f.close();
    return data;

def getTestParams(test_name):
    tmp = test_name.split('_')
    run = 0
    try:
        run = int(tmp[-1])
    except:
        run = 0
    module = "kernel"
    name = []
    for s in tmp:
        if "dev-" in s:
            module = s
            break
        elif s == "KERNEL":
            module = "kernel"
            break
        else:
            name.append(s)
    name = "_".join(name)
    return name, module, run

def averageData(data, key=0, value=[1], bin_size=100, median=True):
    ret = []
    num = len(data)
    cur_x = 0
    idx = [0 for _ in range(num)]
    y = [0 for _ in range(num)]
    width = -1
    while True:
        y = []
        for i in range(num):
             while idx[i] < len(data[i]) and data[i][idx[i]][key] < cur_x:
                  idx[i] += 1
             _idx = idx[i]
             if _idx >= len(data[i]):
                 _idx = len(data[i]) - 1
             if _idx < len(data[i]):
                 if type(value) == list:
                     tmp = []
                     for v in value:
                         tmp.append(data[i][_idx][v])
                     y.append(tmp)
                 else:
                     y.append(data[i][_idx][value])
        if width < 0:
             width = len(y)
        if len(y) == 0 or len(y) < width:
             break;
        if not median:
            ret.append((cur_x, np.average(y, axis=0)))
        else:
            ret.append((cur_x, np.median(y, axis=0)))
        cur_x += bin_size
    return ret

def __cliffsDelta(a, b):
    ret = 0.0
    for va in a:
        for vb in b:
            if va > vb:
                ret += 1.0
            elif va < vb:
                ret -= 1.0
    r = ret / float(len(a) * len(b))
    # print(ret, len(a), len(b), r)
    return r

def cliffsDelta(data0, data1, key=0, value=1, bin_size=30):
    ret = []
    n0 = len(data0)
    n1 = len(data1)
    cur_x = 0
    idx0 = [0 for _ in range(n0)]
    idx1 = [0 for _ in range(n1)]
    while True:
        y0 = []
        y1 = []
        for i in range(n0):
             while idx0[i] < len(data0[i]) and data0[i][idx0[i]][key] < cur_x:
                 idx0[i] += 1
             if idx0[i] < len(data0[i]):
                 y0.append(data0[i][idx0[i]][value])
        for i in range(n1):
             while idx1[i] < len(data1[i]) and data1[i][idx1[i]][key] < cur_x:
                 idx1[i] += 1
             if idx1[i] < len(data1[i]):
                 y1.append(data1[i][idx1[i]][value])
        if len(y0) + len(y1) != n0 + n1:
             break;
        # ret.append((cur_x, np.average(y, axis=0)))
        cd = __cliffsDelta(y0, y1)
        ret.append((cur_x, cd))
        cur_x += bin_size
    return ret
