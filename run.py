#!/usr/bin/python3

import sys
import time
import os
import shutil
import threading
import subprocess
import traceback
import copy
import random
import simplejson as json

from optparse import OptionParser

from python.modules import modules
from python.utils import CUR_DIR, SYZKALLER_DIR, SHELL

blacklist = [
"dev_ashmem",
"dev_binder",
"dev_dri",
"dev_floppy",
"dev_i2c",
"dev_ion",
"dev_kvm",
"dev_ppp",
"dev_ptmx",
"dev_snd_control",
"dev_sr",
"dev_uhid",
"dev_uinput",
"dev_userio",
"dev_video4linux"
]

parser = OptionParser()
parser.add_option("-t", "--type", dest="type", type="string",
                  help="Type of experiment: QEMU/ADB", default="qemu")
parser.add_option("-c", "--config", dest="config", type="string",
                  help="Config file", default="qemu.cfg", metavar="FILE")
parser.add_option("-d", "--duration", dest="duration", type="int",
                  help="Duration of each experiment(s)", default=3600)
parser.add_option("-N", "--num-managers", dest="num_managers", type="int",
                  help="Number of syz-managers fo launch. Only works on QEMU mode", default=1)
parser.add_option("-n", "--num-runs", dest="num_runs", type="int",
                  help="Number of runs for each config/module", default=1)
parser.add_option("-m", "--module", dest="module", type="string",
                  help="Module to fuzz: All/Kernel/name,name,...", default="Kernel")
parser.add_option("-B", "--nobuild", dest="nobuild", action="store_true", help="Do not build syzkaller.", default=False)
parser.add_option("-D", "--nodebug", dest="nodebug", action="store_true", help="Do not keep the debug file.", default=False)

(options, args) = parser.parse_args()
if len(args) >= 1:
    tmp = args[0].strip('.py')
    sys.path.append(os.path.realpath(CUR_DIR))
    config_bases = __import__(tmp).config_bases
else:
    config_bases = __import__("test_template").config_bases
print(config_bases)

if options.type.lower() == "qemu":
    from python.qemu import buildSyzkaller, scheduleTask, workStart
elif options.type.lower() == "adb":
    from python.adb import buildSyzkaller, scheduleTask, workStart
else:
    print("Invalid type")
    exit(1)

modules_to_test = []
if options.module.lower() == "all":
    for m in modules:
        modules_to_test.append(m)
elif options.module.lower() == "kernel":
    modules_to_test = ["KERNEL"]
else:
    tmp = options.module.split(',')
    for m in tmp:
        if m in modules:
            modules_to_test.append(m)
print(modules_to_test)            

tests = []
for cb in config_bases:
  for m in modules_to_test:
    if m in blacklist:
      continue;
    for i in range(options.num_runs):
      cfg = copy.deepcopy(cb)
      cfg["test_name"] = cb["test_name"] + "_%s_%s" % (m.replace('_','-'), str(i).zfill(3))
      cfg["duration"] = options.duration
      cfg["cfg_base"] = options.config
      if m.lower() != "kernel":
        cfg["enable_syscalls"] = modules[m]["enable_syscalls"]
      tests.append(cfg)
random.shuffle(tests)
for t in tests:
    print(t)
    scheduleTask(t)

if not options.nobuild:
    buildSyzkaller(nodedup=False, nodedup_RAMINDEX=False)
if options.type.lower() == "qemu":
    workStart(num_vms=options.num_managers)
else:
    workStart()

# Filter log
for cfg in tests:
    debug_fn = os.path.join(CUR_DIR, "debug_%s" % cfg["test_name"])
    result_fn = os.path.join(CUR_DIR, "result_%s" % cfg["test_name"]) 
    SHELL("%s/filter_log %s %s" % (SYZKALLER_DIR, debug_fn, result_fn))
    if options.nodebug:
        os.remove(debug_fn)
