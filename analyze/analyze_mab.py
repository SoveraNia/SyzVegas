import sys
import os
import copy
import math
import traceback
import simplejson as json
import matplotlib.pyplot as plt
from matplotlib.collections import PatchCollection
from matplotlib.patches import Rectangle
import numpy as np

from utils import loadDataCached, getTestParams, averageData
from plot import plot, plotCDF

def __processTest(test):
    ret = []
    fn = 'result_' + test
    if not os.path.isfile(fn):
        return ret;
    f = open(fn);
    # X-axis
    executeCount = 0; 
    syscallCount = 0;
    ts_bgn = 0;
    # Y-axis
    status = {
        "ts": 0.0,
        "executeCount": 0,
        "syscallCount": 0,
        "MABOverhead": 0.0,
        "MABSync": 0.0,
        "MABUpdate": 0.0,
        "MABPoll": 0.0,
        "MABDequeue": 0.0,
        "MABWeight": [1.0, 1.0, 1.0],
        "MABGLC": [[0.0,0.0,0.0,0.0,0.0,0.0], [0.0,0.0,0.0,0.0,0.0,0.0], [0.0,0.0,0.0,0.0,0.0,0.0]]
    }
    MABGLC = [[[0.0,0.0,0.0,0.0,0.0,0.0]], [[0.0,0.0,0.0,0.0,0.0,0.0]], [[0.0,0.0,0.0,0.0,0.0,0.0]]] # Gen, Mut, Tri, [Gain, Loss, Cost, NormGain, NormLoss, NormCost] 
    idx = 0;
    
    cur_choice = 0
    cur_gain = 0.0
    cur_loss = 0.0
    cur_cost = 0.0
    cur_normgain = 0.0
    cur_normloss = 0.0
    cur_normcost = 0.0
    for line in f:
        line = line.strip('\n').strip();
        if len(line) == 0:
            continue;
        if line[:2] == '- ' and "executeRaw" in line: # Prog/Syscall count
            tmp = line.split();
            try:
                n_calls = int(tmp[-1])
            except:
                n_calls = 1
            #ret.append(copy.deepcopy(status))
            status["executeCount"] += 1;
            status["syscallCount"] += n_calls
        if line[:3] == '<<<' and line[-3:] == '>>>': # Time
            line = line.strip('<<<').strip('>>>')
            ts_cur = int(line)
            if ts_bgn == 0:
                ts_bgn = ts_cur
            status["ts"] = (ts_cur - ts_bgn) / 1000000000
            ret.append(copy.deepcopy(status))
        elif line[0] == '-' and ("MAB Dequeue: " in line or "MAB Update: " in line or "MAB Poll: " in line or "MAB Sync: " in line or "MAB NewTriage: " in line or "MAB CompleteTriage: " in line): # MAB Overhead
            tmp = line.split(": ");
            try:
                t = int(tmp[1]) / 1000000000
            except:
                continue
            status["MABOverhead"] += t
            if "MAB Dequeue: " in line:
                status["MABDequeue"] += t
            elif "MAB Update: " in line:
                status["MABUpdate"] += t
            elif "MAB Poll: " in line:
                status["MABPoll"] += t
            elif "MAB Sync: " in line:
                status["MABSync"] += t
        elif line[0] == '-' and "MABWeight " in line:
            tmp = line.split("MABWeight ")[1].split("], ")[0] + "]"
            try:
                w = json.loads(tmp)
            except:
                continue
            status["MABWeight"] = w
        elif line[0] == '-' and "MAB Choice:" in line:
            cur_choice = int(line.split("MAB Choice: ")[1].split()[0].strip(','))
            #cur_gain = float(line.split("Gain: ")[1].split()[0].strip(','))
            #cur_loss = float(line.split("Loss: ")[1].split()[0].strip(','))
            #cur_cost = float(line.split("Choice: ")[1].split()[0].strip(','))
        elif line[0] == '-' and "MAB Normalized " in line:
            if "Gain: " in line:
                 cur_normgain = float(line.split("Gain: ")[1].split()[0].strip(','))
            if "Loss: " in line:
                 cur_normloss = float(line.split("Loss: ")[1].split()[0].strip(','))
            if "Cost: " in line:
                 cur_normcost = float(line.split("Cost: ")[1].split()[0].strip(','))
            MABGLC[cur_choice].append([cur_gain, cur_loss, cur_cost, cur_normgain, cur_normloss, cur_normcost])
            for i in range(3):
                for j in range(6):
                    status["MABGLC"][i][j] = MABGLC[i][-1][j]
            # ret.append(copy.deepcopy(status))
    f.close();
    return ret, MABGLC;

def plotMAB(tests=["RAMINDEX", "KCOV"]):
    data = {}
    weight = {}
    # Determine whether we should split by module
    splitByModule = False
    for test in tests:
        print(test)
        #if not "MAB" in test:
        #    continue
        try:
            __data, GLC = loadDataCached('mab_%s.cache', test, __processTest);
            print(test, len(__data), __data[-1] if len(__data) > 0 else -1)
            print(GLC[0][-1], GLC[1][-1], GLC[2][-1])
            name, module, run = getTestParams(test)
            print(name, module, run)
            
            if not module in data:
                data[module] = {}
            if not name in data[module]:
                data[module][name] = []
            data[module][name].append(__data);

            # Pring GLC CDF
            '''
            label = ["Gain", "Loss", "Cost", "NormGain", "NormLoss", "NormCost"]
            for i in range(6):
                cdf_data = {
                    "Generate": [v[i] for v in GLC[0]],
                    "Mutate": [v[i] for v in GLC[1]],
                    "Triage": [v[i] for v in GLC[2]],
                }
                plotCDF(cdf_data, xlabel=label[i], outfile="mab_%s_cdf_%s.png" % (label[i].lower(), test))
            '''
        except:
            traceback.print_exc();
            pass;
    for module in data:
        tmp = {}
        keys = ["MABOverhead", "MABSync", "MABDequeue", "MABPoll", "MABUpdate"]
        for k in keys:
            for name in data[module]:
                tmp[name] = averageData(data[module][name], key="ts", value=k)
            plot(tmp, 0, 1, xlabel="Time elapsed (hr)", ylabel=k, outfile="mab_%s_%s.png" % (module, k), xunit=3600.0);
        for name in data[module]:
            # Weight
            tmp = {"Generate": [], "Mutate": [], "Triage": []}
            for i in range(len(data[module][name])):
            #    tmp["Generate"].append([(v["ts"], math.log(v["MABWeight"][0],10)) for v in d])
            #    tmp["Mutate"].append([(v["ts"], math.log(v["MABWeight"][1],10)) for v in d])
            #    tmp["Triage"].append([(v["ts"], math.log(v["MABWeight"][2],10)) for v in d])
            #for arm in tmp:
            #    tmp[arm] = averageData(tmp[arm], key=0, value=1)
                d = data[module][name][i]
                tmp = {
                     "Generate": [(v["ts"], math.log(v["MABWeight"][0],10)) for v in d], 
                     "Mutate": [(v["ts"], math.log(v["MABWeight"][1],10)) for v in d], 
                     "Triage": [(v["ts"], math.log(v["MABWeight"][2],10)) for v in d]
                }
                plot(tmp, 0, 1, xlabel="Time elapsed (hr)", ylabel="log(Weight)", title="MAB Weight", outfile="mab_weight_%s_%s_%d.png" % (name, module, i), xunit=3600.0);
            # GLC
            '''
            label = ["Gain", "Loss", "Cost", "NormGain", "NormLoss", "NormCost"]
            for i in range(3):
                tmp = {"Generate": [], "Mutate": [], "Triage": []}
                for d in data[module][name]:
                    tmp["Generate"].append([(v["ts"], v["MABGLC"][0][i]) for v in d])
                    tmp["Mutate"].append([(v["ts"], v["MABGLC"][1][i]) for v in d])
                    tmp["Triage"].append([(v["ts"], v["MABGLC"][2][i]) for v in d])
                for arm in tmp:
                    tmp[arm] = averageData(tmp[arm], key=0, value=1)
                plot(tmp, 0, 1, xlabel="Time elapsed (hr)", ylabel=label[i], title="MAB %s" % label[i], outfile="mab_%s_%s_%s.png" % (label[i].lower(), name, module), ylogscale=False);
            '''



