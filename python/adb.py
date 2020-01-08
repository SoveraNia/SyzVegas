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
devices = {
#    "84B7N16219002600": None,
    "84B7N15A28012929": None
}
workqueue = [];
from python.utils import CUR_DIR, SYZKALLER_DIR, SHELL, getOpenPort, filterLog, setEnv

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
    SHELL("make TARGETOS=linux TARGETARCH=arm64 CFLAGS='%s'" % cflags)
    os.chdir(CUR_DIR)

def createCfg(cfg_base="adb.cfg", feedback="RAMINDEX", test_name="0", exp_id=0, dev_id="84B7N16219002600", fuzzer_config={}, enable_syscalls=None):
    cfg_fn = "tmp_%s.cfg" % test_name
    fr = open(cfg_base);
    data = json.load(fr);
    fr.close()
    cur_dir = os.path.dirname(os.path.realpath(__file__))
    data["workdir"] = "%s/workdir_%s" % (CUR_DIR, test_name)
    data["syzkaller"] = SYZKALLER_DIR
    # port = 50000 + exp_id % 100
    port = getOpenPort()
    data["target"] = "linux/arm64"
    data["http"] = "localhost:%u" % port
    data["vm"] = { "devices": [dev_id] }
    data["feedback"] = feedback;
    if not enable_syscalls is None:
        data["enable_syscalls"] = enable_syscalls
    data["fuzzer_config"] = {}
    for key in fuzzer_config:
        # if key in ["signalRunThreshold", "noMinimization", "executeRetries", "noiseInjection"]:
        data["fuzzer_config"][key] = fuzzer_config[key]
    print(data);
    fw = open(cfg_fn, "w+")
    json.dump(data, fw);
    fw.close();
    return cfg_fn, data

def runExperiment(exp_id=None, test_name=None, cfg_base="adb.cfg", dev_id="84B7N16219002600", feedback="RAMINDEX", duration=600, fuzzer_config={}, nodedup=False, rebuild=False, debug=True):
    if exp_id is None:
        exp_id = time.time() * 1000000000
    if test_name is None:
        test_name = "%u_%s" % (exp_id, feedback)
    print(exp_id, test_name, feedback, duration, fuzzer_config)

    # Rebuild
    if rebuild:
        buildSyzkaller()
    cfg, cfg_data = createCfg(cfg_base=cfg_base, feedback=feedback, dev_id=dev_id, test_name=test_name, exp_id=exp_id, fuzzer_config=fuzzer_config)

    # Cleaning up
    SHELL("rm -fr %s/*" % cfg_data["workdir"], permissive=True)
    SHELL("adb -s %s shell su -c rm /data/local/tmp/debug.log" % dev_id, permissive=True)
    SHELL("adb -s %s shell su -c rm -fr /data/local/tmp/syzlog*" % dev_id, permissive=True)
    SHELL("adb -s %s shell mkdir /data/local/tmp/syzlog" % dev_id, permissive=True)

    # Run
    out_fp = open("log_%s" % test_name, "w+")
    debug_fp = None
    cmd = "%s/bin/syz-manager -config %s" % (SYZKALLER_DIR, cfg)
    debug_fp = open("debug_%s" % test_name, "w+")
    if debug:
        cmd += " -debug"
    try:
        p = subprocess.run(cmd.split(), stdout=debug_fp, stderr=out_fp, timeout=duration)
    except subprocess.TimeoutExpired:
        pass;
    out_fp.close()
    if not debug_fp is None:
        debug_fp.close();
    '''
    time.sleep(duration)
    try:
        #os.killpg(os.getpgid(p.pid), signal.SIGKILL)
        p.terminate();
        p.terminate();
        p.kill();
    except:
        traceback.print_exc();
    '''

    # Pull logs
    #SHELL("adb -s %s pull /data/local/tmp/syzlog syzlog_%s" % (dev_id, test_name), permissive=True)
    #SHELL("adb -s %s pull /data/local/tmp/syzlog.log syzlog_%s.log" % (dev_id, test_name), permissive=True)
    #SHELL("adb -s %s pull /data/local/tmp/debug.log syscalls_%s" % (dev_id, test_name), permissive=True)
    shutil.move("%s/corpus.db" % cfg_data["workdir"], "corpus_%s.db" % test_name)
    SHELL("%s/bin/syz-db unpack corpus_%s.db corpus_%s" % (SYZKALLER_DIR, test_name, test_name), permissive=True)

    # os.remove(cfg)
    time.sleep(60)

def killSyzkaller():
    SHELL("killall syz-manager", permissive=True)
    time.sleep(1)
    SHELL("killall syz-manager", permissive=True)
    time.sleep(1)
    SHELL("killall syz-manager", permissive=True)

def scheduleTask(kwargs):
    workqueue.append(kwargs);

def workStart():
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
            elif devices[dev] is not None and devices[dev].isAlive():
                done = False
        time.sleep(10)
        sys.stdout.write('.')
        if done:
            return

if __name__ == "__main__":
    duration = 600
    buildSyzkaller(nodedup=False, nodedup_RAMINDEX=True);
    '''
    scheduleTask({'test_name':'KCOV_100-100', 'feedback':"KCOV", 'duration':duration, 'fuzzer_config':{'executeRetries': 0, 'noMinimization': False, 'mutateWeight': 100, 'smashWeight': 100, "signalRunThreshold": 0.0}})
    scheduleTask({'test_name':'KCOV_25-25', 'feedback':"KCOV", 'duration':duration, 'fuzzer_config':{'executeRetries': 0, 'noMinimization': False, 'mutateWeight': 25, 'smashWeight': 25, "signalRunThreshold": 0.0}})
    scheduleTask({'test_name':'KCOV_10-10', 'feedback':"KCOV", 'duration':duration, 'fuzzer_config':{'executeRetries': 0, 'noMinimization': False, 'mutateWeight': 10, 'smashWeight': 10, "signalRunThreshold": 0.0}})
    scheduleTask({'test_name':'KCOV_1-1', 'feedback':"KCOV", 'duration':duration, 'fuzzer_config':{'executeRetries': 0, 'noMinimization': False, 'mutateWeight': 1, 'smashWeight': 1, "signalRunThreshold": 0.0}})
    '''
    scheduleTask({'test_name':'RAMINDEX_100-100', 'feedback':"RAMINDEX", 'duration':duration, 'fuzzer_config':{'executeRetries': 0, 'noMinimization': False, 'mutateWeight': 100, 'smashWeight': 100, "signalRunThreshold": 0.0}})
    scheduleTask({'test_name':'RAMINDEX_25-25', 'feedback':"RAMINDEX", 'duration':duration, 'fuzzer_config':{'executeRetries': 0, 'noMinimization': False, 'mutateWeight': 25, 'smashWeight': 25, "signalRunThreshold": 0.0}})
    #scheduleTask({'test_name':'RAMINDEX_10-10', 'feedback':"RAMINDEX", 'duration':duration, 'fuzzer_config':{'executeRetries': 0, 'noMinimization': False, 'mutateWeight': 10, 'smashWeight': 10, "signalRunThreshold": 0.0}})
    #scheduleTask({'test_name':'RAMINDEX_1-1', 'feedback':"RAMINDEX", 'duration':duration, 'fuzzer_config':{'executeRetries': 0, 'noMinimization': False, 'mutateWeight': 1, 'smashWeight': 1, "signalRunThreshold": 0.0}})
    print("Starting thread")
    workStart();
    print("Success")

