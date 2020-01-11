import sys
import os
import copy
import glob
import traceback
import simplejson as json
import matplotlib.pyplot as plt
from matplotlib.collections import PatchCollection
from matplotlib.patches import Rectangle
import numpy as np
import pygraphviz as PG

from plot import plot, plotBar, plotCDF, plotBar1
from utils import loadDataCached, getTestParams

def __processCrashLog(fn):
    ret = {
        "prog": None,
        "description": "", 
        "ts": 0
    }
    f = open(fn)
    for line in f:
        line = line.strip('\n').strip();
    f.close();

def __processTest(test):
    ret = []
    workdir = 'workdir_' + test
    if not os.path.isdir(workdir):
        return None
    flist = glob.glob()
    for line in f:
        line = line.strip('\n').strip();
    f.close();
    return p_all, p_generated, p_corpus, p_triage, ret;

def plotCrashes(tests=["KCOV", "RAMINDEX"]):
    datas = {}
    for test in tests:
        name, module, run = getTestParams(test)
        name = name + '_' + module

    for test in tests:
        name, module, run = getTestParams(test)
        name = name + '_' + module
        print("Plotting crashes for %s" % test)
        try:
            # p_all, p_generated, p_corpus, p_triage, __data = __processTest(test);
            __data = loadDataCached('crashes_%s.cache', test, __processTest);
        except:
            traceback.print_exc()
            continue;

