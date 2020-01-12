import sys
import os
import copy
import glob
import traceback
import shutil
import simplejson as json
import matplotlib.pyplot as plt
from matplotlib.collections import PatchCollection
from matplotlib.patches import Rectangle
import numpy as np
import pygraphviz as PG

from plot import plot, plotBar, plotCDF, plotBar1, sortKeys
from utils import loadDataCached, getTestParams, filterLog, averageData, cliffsDelta

def __getStartTime(test):
    inslock = 'workdir_' + test + '/instance-lock'
    ts = 0.0
    stat = os.stat(inslock)
    # print(stat)
    ts_stat = stat.st_mtime

    fn = "result_" + test
    ts = 0.0
    f = open(fn)
    for line in f:
        line = line.strip('\n')
        if line[:3] == '<<<' and line[-3:] == '>>>':
            line = line.strip('<<<').strip('>>>')
            ts = int(line)
            break
    ts_log = ts / 1000000000.0
    print(ts_stat, ts_log)
    if abs(ts_stat - ts_log) > 60:
        print("Error:", test)
        exit(1)
    return ts_stat

def __processCrashLog(fn, tbgn=0.0):
    ret = {
        "prog": "",
        "ts": 0
    }
    inProg = False
    f = open(fn)
    for line in f:
        line = line.strip('\n').strip();
        if line[:3] == "<<<" and line[-3:] == ">>>":
            line = line.strip('<<<').strip('>>>')
            ts = int(line)
            ret["ts"] = ts / 1000000000.0 - tbgn
        elif 'executing program' in line:
            inProg = True
            ret["prog"] = ""
        elif '- executeRaw' in line:
            inProg = False
        elif inProg:
            ret["prog"] += line + '\n'
    f.close();
    return ret

def __processTest(test):
    stats = []
    ret = {}
    workdir = 'workdir_' + test
    if not os.path.isdir(workdir):
        return None
    tbgn = __getStartTime(test)
    crash_id_list = glob.glob(workdir + '/crashes/*')
    for __cid in crash_id_list:
        cid = __cid.split('/')[-1]
        print(cid)
        ret[cid] = {
            "Description": "",
            "ts": -1,
            "Crashes": []
        }
        # Description
        desc_fn = workdir + '/crashes/' + cid + '/description'
        desc = ""
        if os.path.isfile(desc_fn):
            with open(desc_fn, 'r') as f:
                desc = f.read()
        ret[cid]["Description"] = desc
        ret[cid]["ts"] = os.stat(desc_fn).st_mtime - tbgn
        # print(os.stat(desc_fn))
        print(desc, ret[cid]["ts"])
        # Actual crashes
        crash_log_list = glob.glob(workdir + '/crashes/' + cid + "/log*") 
        log_idx = 0
        for log_fn in crash_log_list:
            if 'filtered' in log_fn:
                continue
            report_fn = log_fn.split('/')[-1].replace('log', 'report')
            log_idx = report_fn.strip('report')
            report_fn = workdir + '/crashes/' + cid + '/' + report_fn
            prog_fn = workdir + '/crashes/' + cid + '/prog' + log_idx
            if os.path.isfile(log_fn):
                log_fn_filtered = log_fn + '_filtered'
                if not os.path.isfile(log_fn_filtered):
                    filterLog(log_fn, log_fn_filtered)
                cr = __processCrashLog(log_fn_filtered, tbgn=tbgn)
                # Write prog file
                with open(prog_fn, 'w+') as f:
                    f.write(cr["prog"])
                cr["log_fn"] = log_fn
                cr["report_fn"] = report_fn
                ret[cid]["Crashes"].append(cr)
                #if ret[cid]["ts"] < 0 or ret[cid]["ts"] > cr["ts"]:
                #    ret[cid]["ts"] = cr["ts"]
        print(ret[cid]["ts"])
    return ret;

def buildCrashDb(datas):
    db = {}
    if not os.path.isdir("crash-db"):
        os.mkdir("crash-db")
    for name in datas:
      for d in datas[name]:
        for csig in d:
          print(d[csig]["Description"])
          desc = d[csig]["Description"].strip('\n')
          crash_dir = "crash-db/" + desc.replace(' ', '_').replace('\n', '_')
          if not desc in db:
              db[desc] = {
                  "Discover_Count": {},
                  "Sigs": {},
                  "Crashes": [],
              }
              for n in datas:
                  db[desc]["Discover_Count"][n] = 0
              if not os.path.isdir(crash_dir):
                  os.mkdir(crash_dir)
          db[desc]["Discover_Count"][name] += 1
          if not csig in db[desc]["Sigs"]:
              db[desc]["Sigs"][csig] = 0
          db[desc]["Sigs"][csig] += 1
          for c in d[csig]["Crashes"]:
              cidx = len(db[desc]["Crashes"])
              db[desc]["Crashes"].append(c)
              # Copy prog and report to new folder
              log_fn_old = c["log_fn"]
              report_fn_old = c["report_fn"]
              prog = c["prog"]
              log_fn_new = crash_dir + '/log' + str(cidx).zfill(3)
              report_fn_new = crash_dir + '/report' + str(cidx).zfill(3)
              prog_fn = crash_dir + '/prog' + str(cidx).zfill(3)
              try:
                  shutil.copy(log_fn_old, log_fn_new)
                  shutil.copy(report_fn_old, report_fn_new)
              except:
                  traceback.print_exc()
                  pass
              with open(prog_fn, 'w+') as f:
                  f.write(c["prog"])
    # Print a table of crashes discovery
    names = sortKeys(datas.keys())
    print("Discover Count")
    print('\t' + '\t'.join(names))
    for desc in db:
        out = desc
        for n in names:
            out += '\t%d' % db[desc]["Discover_Count"][n]
        print(out)
    return db

def plotCrashes(tests=["KCOV", "RAMINDEX"]):
    datas = {}
    for test in tests:
        name, module, run = getTestParams(test)
        name = name + '_' + module
        if not name in datas:
            datas[name] = []

    for test in tests:
        name, module, run = getTestParams(test)
        name = name + '_' + module
        print("Plotting crashes for %s" % test)
        __data = None
        try:
            # p_all, p_generated, p_corpus, p_triage, __data = __processTest(test);
            __data = loadDataCached('crashes_%s.cache', test, __processTest);
            #__data = __processTest(test)
            datas[name].append(__data)
        except:
            traceback.print_exc()
            continue;

    buildCrashDb(datas)

    # Crash count
    crash_count = {}
    crashes_avg = {}
    for name in datas:
        crash_count[name] = []
        for d in datas[name]:
            ts = []
            for cid in d:
                ts.append(d[cid]["ts"])
            ts.sort()
            tmp = [(0,0)] + [(ts[i], i+1) for i in range(len(ts))]
            crash_count[name].append(tmp)
        crashes_avg[name] = averageData(crash_count[name], key=0, value=1, bin_size=600)
        # print(crash_count[name])
    plot(crashes_avg, 0, 1, xlabel="Time elapsed (hr)", ylabel="Crahses Found", title="", outfile="crashes_time.png", xunit=3600.0, nmarkers=12, xstep=4);
    # Cliff's delta
    tmp = {}
    for name0 in crash_count:
        if "Default" in name0:
            continue
        for name1 in crash_count:
            if not "Default" in name1:
                continue
            tmp[name0] = cliffsDelta(crash_count[name0], crash_count[name1], key=0, value=1, bin_size=600)
    plot(tmp, 0, 1, xlabel="Time elapsed (hr)", ylabel="Cliff's Delta", outfile="crashes_cd_time.png", xunit=3600.0, nmarkers=12, xstep=4);
