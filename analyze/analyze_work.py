import sys
import os
import copy
import traceback
import simplejson as json
import matplotlib.pyplot as plt
from matplotlib.collections import PatchCollection
from matplotlib.patches import Rectangle
import numpy as np

from plot import plot, plotBar1
from utils import loadDataCached, getTestParams, averageData

keys = ["Generate", "signalRun", "Minimize", "Mutate", "Triage Total"]

def __parseResult(s):
    ret = {}
    tmp = s.strip().strip('{').strip('}').split()
    for d in tmp:
        if ':' in d:
            d = d.split(':')
            k = d[0]
            if d[1] == "true":
                v = True
            elif d[1] == "false":
                v = False
            else:
                try: # Ignore non-numeric fields
                    v = float(d[1])
                except:
                    continue
            ret[k] = v
    return ret

def __flattenStatus(s):
    ret = {}
    for k in s:
        if type(s[k]) != list:
            ret[k] = s[k]
        elif len(s[k]) == 3:
            ret[k+"_Generate"] = s[k][0]
            ret[k+"_Mutate"] = s[k][1]
            ret[k+"_Triage"] = s[k][2]
            ret[k+"_All"] = s[k][0] + s[k][1] + s[k][2]
    return ret

def __processTest(test):
    ret = []
    fn = 'result_' + test
    if not os.path.isfile(fn):
        return ret;
    t_bgn = -1;
    cur_func = -1
    exec_count = 0
    syscall_count = 0
    TIME_THRESHOLD = 10000.0
    prev_ts = 0
    cur_ts = 0
    cur_status = {
        "Time_Elapsed": 0.0,
        "Works_Done": [0,0,0],
        "Execute_Time": [0.0, 0.0, 0.0],
        "Total_Time": [0.0, 0.0, 0.0],
        "Syscalls_Made": [0, 0, 0],
        "Programs_Executed": [0, 0, 0],
        "Triages_Failed": 0
    }
    exec_time = [[], [], []]
    f = open(fn)
    for line in f:
        line = line.strip('\n').strip();
        if len(line) == 0:
            continue;
        if line[:3] == "<<<" and line[-3:] == ">>>":
            ts = int(line.strip("<<<").strip(">>>"))
            if t_bgn < 0:
                t_bgn = ts;
            ts = ts - t_bgn
            cur_status["Time_Elapsed"] = ts / 1000000000.0
            cur_ts = cur_status["Time_Elapsed"]
            ret.append(__flattenStatus(cur_status))
        elif ("MAB Choice: " in line or "Work Type: " in line):
            TIME_THRESHOLD = cur_ts - prev_ts + 10.0 # Upper bound
            try:
              if "Work Type: " in line:
                cur_func = int(line.split("Work Type: ")[1].split(',')[0])
              elif "MAB Choice: " in line:
                cur_func = int(line.split("MAB Choice: ")[1].split(',')[0])
            except:
              print("WTF", line)
              continue
            cur_status["Works_Done"][cur_func] += 1
            if "Result: " in line:
                d = line.split("Result: ")[1]
                d = __parseResult(d)
                _ttot = 0
                if "timeTotal" in d:
                    _ttot = d["timeTotal"] / 1000.0
                _ttot = _ttot if _ttot < TIME_THRESHOLD else TIME_THRESHOLD
                _ttot = _ttot if _ttot > 0.0 else 0.0
                cur_status["Total_Time"][cur_func] += _ttot
                if cur_func == 2: # Triage: Combine minimize and verify together
                    _texc = 0
                    if "minimizeTime" in d and "verifyTime" in d:
                        _texc = (d["minimizeTime"] + d["verifyTime"]) / 1000.0
                    _texc = _texc if _texc < TIME_THRESHOLD else TIME_THRESHOLD
                    _texc = _texc if _texc > 0.0 else 0.0
                    cur_status["Execute_Time"][2] += _texc
                    exec_time[2].append(_texc)
                    if "success" in d and not d["success"]:
                        cur_status["Triages_Failed"] += 1
                else:
                    _texc = 0
                    if "time" in d:
                        _texc = (d["time"]) / 1000.0
                    _texc = _texc if _texc < TIME_THRESHOLD else TIME_THRESHOLD
                    _texc = _texc if _texc > 0.0 else 0.0
                    exec_time[cur_func].append(_texc)
                    cur_status["Execute_Time"][cur_func] += _texc
            cur_status["Programs_Executed"][cur_func] += exec_count
            cur_status["Syscalls_Made"][cur_func] += syscall_count
            exec_count = 0
            syscall_count = 0
            prev_ts = cur_status["Time_Elapsed"]
        elif line[0] == '-' and 'executeRaw' in line:
            exec_count += 1;
            try:
                sz = int(line.split()[-1])
                #if cur_func in sizes:
                #    sizes[cur_func].append(sz)
                syscall_count += sz;
            except:
                sz = 0
    f.close();
    return ret, exec_time;

def __plotWork(test):
    # __data = __processTest(test);
    __data, exec_time = loadDataCached("work_%s.cache", test, __processTest);
    if len(__data) <= 1:
        return;
    # Execute time percentile
    for i in range(3):
        print(i,
              np.percentile(exec_time[i], 50), np.percentile(exec_time[i], 75), np.percentile(exec_time[i], 90), np.percentile(exec_time[i], 95), np.percentile(exec_time[i], 99),
              2 * np.percentile(exec_time[i], 75) - np.percentile(exec_time[i], 25)
              )
    exec_time_all = exec_time[0] + exec_time[1] + exec_time[2]
    print("All",
              np.percentile(exec_time_all, 50), np.percentile(exec_time_all, 75), np.percentile(exec_time_all, 90), np.percentile(exec_time_all, 95), np.percentile(exec_time_all, 99),
              2 * np.percentile(exec_time_all, 75) - np.percentile(exec_time_all, 25)
              )
    data = {};
    for i,key in enumerate(keys):
        data[key] = []
        for j in range(len(__data)):
            # if __data[j][0] < 350000:
                data[key].append((__data[j][0], __data[j][1], __data[j][i+2]))
    #plot(data, 0, 2, xlabel="# of executions", ylabel="# of executions", title="", outfile="work_%s.png" % test, ylogscale=False);
    plot(data, 1, 2, xlabel="Time elapsed (hr)", ylabel="# of executions", title="", outfile="work_time_%s.png" % test, ylogscale=False, xunit=3600.0);

def __plotWorkDist(data, key, module="", ylogscale=False, ylabel=""):
    # Programs Executed
    tmp = {}
    for name in data:
      for job in ["Generate", "Mutate", "Triage"]:
        tmp[name + "_" + job] = averageData(data[name], key="Time_Elapsed", value=key + "_" + job, bin_size=10)
        plot(tmp, 0, 1, xlabel="Time elapsed (hr)", ylabel=ylabel, outfile="work_%s_%s.png" % (module, key), ylogscale=ylogscale, xunit=3600.0);
    tmp = {}
    for name in data:
      tmp[name] = {}
      for job in ["Generate", "Mutate", "Triage"]:
        tmp[name][job] = []
        for d in data[name]:
          tmp[name][job].append(d[-1][key + "_" + job])
    plotBar1(tmp, ylabel=ylabel, outfile="work_%s_%s_bar.png" % (module, key), ylogscale=ylogscale);


def plotWork(tests=["KCOV", "RAMINDEX"]):
    '''
    for test in tests:
      try:
        __plotWork(test)
      except:
        traceback.print_exc()
    '''
    modules = {}
    # Determine whether we should split by module
    for test in tests:
        print(test)
        name, module, run = getTestParams(test)
        if not module in modules:
            modules[module] = []
        modules[module].append(test)
    for module in modules:
        data = {}
        exec_time = []
        for test in modules[module]:
          try:
            __data, exec_time = loadDataCached('work_%s.cache', test, __processTest);
            print(test, len(__data), __data[-1] if len(__data) > 0 else -1)
            name, module, run = getTestParams(test)
            print(name, module, run)
            if not name in data:
                data[name] = []
            data[name].append(__data);
            # Time distribution
            tmp = {
                "Median": {},
                #"99 Percentile": {},
                "Q3+IQR": {},
            }
            __keys = ["Generate", "Mutate", "Triage", "All"]
            exec_time.append(exec_time[0] + exec_time[1] + exec_time[2])
            for i in range(4):
                 tmp["Median"][__keys[i]] = np.percentile(exec_time[i], 50)
                 #tmp["99 Percentile"][__keys[i]] = np.percentile(exec_time[i], 99)
                 tmp["Q3+IQR"][__keys[i]] = 2 * np.percentile(exec_time[i], 75) + np.percentile(exec_time[i], 25)
            print(tmp)
            plotBar1(tmp, ylabel="Time (s)", outfile="work_time_percentile_%s.png" % test)
          except:
            traceback.print_exc()
            continue


        # Average / median result
        # Time Total
        tmp = {}
        for name in data:
          for job in ["Generate", "Mutate", "Triage"]:
            tmp[name + "_" + job] = averageData(data[name], key="Time_Elapsed", value="Total_Time_" + job, bin_size=10)
        plot(tmp, 0, 1, xlabel="Time elapsed (hr)", ylabel="Time (s)", outfile="work_%s_time_total.png" % module, ylogscale=False, xunit=3600.0, nmarkers=12);
        tmp = {}
        for name in data:
            tmp[name + "_All"] = averageData(data[name], key="Time_Elapsed", value="Total_Time_All", bin_size=10)
        plot(tmp, 0, 1, xlabel="Time elapsed (hr)", ylabel="Time (s)", outfile="work_%s_time_total_all.png" % module, ylogscale=False, xunit=3600.0, nmarkers=12);
        # Execute Time
        tmp = {}
        for name in data:
          for job in ["Generate", "Mutate", "Triage"]:
            tmp[name + "_" + job] = averageData(data[name], key="Time_Elapsed", value="Execute_Time_" + job, bin_size=10)
        plot(tmp, 0, 1, xlabel="Time elapsed (hr)", ylabel="Time (s)", outfile="work_%s_time_execute.png" % module, ylogscale=False, xunit=3600.0, nmarkers=12);
        tmp = {}
        for name in data:
            tmp[name + "_All"] = averageData(data[name], key="Time_Elapsed", value="Execute_Time_All", bin_size=10)
        plot(tmp, 0, 1, xlabel="Time elapsed (hr)", ylabel="Time (s)", outfile="work_%s_time_execute_all.png" % module, ylogscale=False, xunit=3600.0, nmarkers=12);
        # Overall Choices
        __plotWorkDist(data, module=module, key="Works_Done", ylabel="Choice", ylogscale=True)
        # Syscalls made
        #tmp = {}
        #for name in data:
        #  for job in ["Generate", "Mutate", "Triage"]:
        #    tmp[name + "_" + job] = averageData(data[name], key="Time_Elapsed", value="Syscalls_Made_" + job, bin_size=10)
        #plot(tmp, 0, 1, xlabel="Time elapsed (hr)", ylabel="Syscalls", outfile="work_%s_syscalls.png" % module, ylogscale=False);
        # Programs Executed
        __plotWorkDist(data, module=module, key="Programs_Executed", ylabel="Programs", ylogscale=True)


