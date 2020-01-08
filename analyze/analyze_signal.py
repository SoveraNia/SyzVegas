import sys
import os
import simplejson as json
import matplotlib.pyplot as plt
from matplotlib.collections import PatchCollection
from matplotlib.patches import Rectangle
import numpy as np

from plot import plot

def __processTest(test):
    ret = []
    fn = 'syscalls_' + test
    if not os.path.isfile(fn):
        return ret;
    count = 0;
    t_bgn = -1;
    prev_ts = -1
    f = open(fn)
    for line in f:
        line = line.strip('\n').strip();
        if len(line) == 0:
            continue;
        if line[0] == '-':
            tmp = line.split();
            try:
                ts = int(tmp[1])
            except:
                tmp[1] = tmp[1].lower().strip('%!u(int64=').strip(')')
                try:
                    ts = int(tmp[1])
                except:
                    ts = prev_ts + t_bgn
            if t_bgn < 0:
                t_bgn = ts;
            prev_ts = ts - t_bgn;
            count += 1;
        elif "checkNewCallSignal" in line:
            tmp = line.split();
            if len(tmp) < 6:
                continue; 
            maxSigStr = tmp[4];
            corpusSigStr = tmp[5];
            _tmp = maxSigStr.strip('max:').split(',')
            maxSigTarget = int(_tmp[2])
            maxSigKCOV = int(_tmp[3])
            _tmp = corpusSigStr.strip('corpus:').split(',')
            corpusSigTarget = int(_tmp[2])
            corpusSigKCOV = int(_tmp[3])
            ret.append((count, prev_ts, maxSigTarget, maxSigKCOV, corpusSigTarget, corpusSigKCOV))
    f.close();
    return ret;

def plotSignal(tests=["KCOV", "RAMINDEX"]):
  for test in tests:
    __data = __processTest(test);
    if len(__data) <= 1:
      continue;
    data = {
	"MaxSignalTarget": [(__data[i][0], __data[i][1], __data[i][2]) for i in range(len(__data))],
	"MaxSignalKCOV": [(__data[i][0], __data[i][1], __data[i][3]) for i in range(len(__data))],
        "CorpusSignalTarget": [(__data[i][0], __data[i][1], __data[i][4]) for i in range(len(__data))],
        "CorpusSignalKCOV": [(__data[i][0], __data[i][1], __data[i][5]) for i in range(len(__data))],
    }
    plot(data, 1, 2, xlabel="Time elapsed (s)", ylabel="# of signals", title=test, outfile="signal_time_%s.png" % test);
