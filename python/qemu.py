#!/usr/bin/python3

import sys
import time
import os
import shutil
import threading
import subprocess
import traceback
import simplejson as json

# UCI device: 84B7N16219002600
workqueue = [];
from python.utils import CUR_DIR, SYZKALLER_DIR, SHELL, getOpenPort, setEnv, filterLog

def buildSyzkaller(nodedup=False, nodedup_RAMINDEX=False):
    setEnv()
    os.chdir(SYZKALLER_DIR)
    SHELL("make clean")
    SHELL("make generate")
    cflags = '-D DWANG030_ALL'
    if nodedup:
        cflags += ' -D DWANG030_NODEDUP'
    if nodedup_RAMINDEX:
        cflags += ' -D DWANG030_NODEDUP_RAMINDEX'
    SHELL("make TARGETOS=linux CFLAGS='%s'" % cflags)
    os.chdir(CUR_DIR)

def createCfg(cfg_base="qemu.cfg", feedback="RAMINDEX", test_name="0", exp_id=0, dev_id="84B7N16219002600", fuzzer_config={}, enable_syscalls=None):
    cfg_fn = "tmp_%s.cfg" % test_name
    fr = open(cfg_base);
    data = json.load(fr);
    fr.close()
    port = getOpenPort()
    data["http"] = "localhost:%u" % port
    data["feedback"] = feedback;
    data["workdir"] = "%s/workdir_%s" % (CUR_DIR, test_name)
    data["fuzzer_config"] = {}
    data["syzkaller"] = SYZKALLER_DIR
    if not enable_syscalls is None:
        data["enable_syscalls"] = enable_syscalls
    #if not "disable_syscalls" in data:
    #    data["disable_syscalls"] = []
    #data["disable_syscalls"].append("clock_settime")
    for key in fuzzer_config:
        #if key in ["signalRunThreshold", "noMinimization", "executeRetries", "noiseInjection", ]:
        data["fuzzer_config"][key] = fuzzer_config[key]
    print(data);
    fw = open(cfg_fn, "w+")
    json.dump(data, fw);
    fw.close();
    return cfg_fn, data

def runExperiment(exp_id=None, test_name=None, cfg_base="qemu.cfg", dev_id="84B7N16219002600", feedback="RAMINDEX", duration=600, fuzzer_config={}, nodedup=False, debug=True, enable_syscalls=None):
    if exp_id is None:
        exp_id = time.time() * 1000000000
    if test_name is None:
        test_name = "%u_%s" % (exp_id, feedback)
    print(exp_id, test_name, feedback, debug, duration, fuzzer_config)

    cfg, cfg_data = createCfg(cfg_base=cfg_base, test_name=test_name, feedback=feedback, dev_id=dev_id, exp_id=exp_id, fuzzer_config=fuzzer_config, enable_syscalls=enable_syscalls)

    # Cleaning up
    SHELL("rm -fr %s/*" % cfg_data["workdir"], permissive=True)

    # Run
    log_fn = os.path.join(CUR_DIR, "log_%s" % test_name)
    out_fp = open(log_fn, "w+")
    debug_fn = os.path.join(CUR_DIR, "debug_%s" % test_name)
    debug_fp = None
    result_fn = os.path.join(CUR_DIR, "result_%s" % test_name)
    cmd = "%s/bin/syz-manager -config %s" % (SYZKALLER_DIR, cfg)
    debug_fp = open(debug_fn, "w+")
    if debug:
        cmd += " -debug"
    try:
        p = subprocess.run(cmd.split(), stdout=debug_fp, stderr=out_fp, timeout=duration)
    except subprocess.TimeoutExpired:
        pass;
    except:
        print("WTF")
    out_fp.close()
    if not debug_fp is None:
        debug_fp.close();

    # Pull logs
    shutil.move("%s/corpus.db" % cfg_data["workdir"], "%s/corpus_%s.db" % (CUR_DIR, test_name))
    SHELL("%s/bin/syz-db unpack corpus_%s.db corpus_%s" % (SYZKALLER_DIR, test_name, test_name), permissive=True)
    #SHELL("grep -v '^\[' %s > %s.tmp" % (debug_fn, result_fn))
    #SHELL("mv %s.tmp %s" % (result_fn, result_fn))i
    #filterLog(debug_fn, result_fn)
    #SHELL("%s/filter_log %s %s" % (SYZKALLER_DIR, debug_fn, result_fn))

    # os.remove(cfg)
    time.sleep(10)

def killSyzkaller():
    SHELL("killall syz-manager", permissive=True)
    time.sleep(1)
    SHELL("killall syz-manager", permissive=True)
    time.sleep(1)
    SHELL("killall syz-manager", permissive=True)

def scheduleTask(kwargs):
    workqueue.append(kwargs);

def workStart(num_vms=1):
    devices = {}
    for i in range(num_vms):
        devices["VM%d" % i] = None
    exp_id = 0;
    while True:
        done = True
        for dev in devices:
            if (devices[dev] is None or not devices[dev].isAlive()) and len(workqueue) > 0:
                if not devices[dev] is None:
                    devices[dev].join();
                task = workqueue.pop(0)
                task['dev_id'] = dev
                task['exp_id'] = exp_id;
                exp_id += 1;
                devices[dev] = threading.Thread(target=runExperiment, kwargs=task)
                devices[dev].start()
                print("Starting task %s on device %s" % (task, dev))
                done = False
                break # Wait for the next tick for the task to occupy the port
            elif devices[dev] is not None and devices[dev].isAlive():
                done = False
        # Sanity check to prevent disk overflow
        total, used, free = shutil.disk_usage(__file__)
        if free < 10 * (2 ** 30):
            print("Error: running out of disk space. Abort!")
            killSyzkaller()
            exit(1)
        time.sleep(30)
        sys.stdout.write('.')
        sys.stdout.flush()
        if done:
            break
