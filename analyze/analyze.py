#!/usr/bin/python3

import sys
import os
import math;
import glob;
import traceback
import matplotlib.pyplot as plt
from matplotlib.collections import PatchCollection
from matplotlib.patches import Rectangle
import numpy as np
import simplejson as json
from optparse import OptionParser

from analyze_debug import parseDebug
from analyze_work import plotWork
from analyze_corpus import plotCorpus
from analyze_signal import plotSignal
from analyze_coverage import plotCoverage
from analyze_programs import plotPrograms
from analyze_triage import plotTriage
from analyze_mab import plotMAB
from analyze_seeds import plotSeeds
from analyze_mutationtree import plotMutationTree
from analyze_crashes import plotCrashes
from plot import plot

if __name__ == "__main__":
    # tests = ["RAMINDEX-0.0", "RAMINDEX-0.2", "RAMINDEX-0.4", "RAMINDEX-0.6", "RAMINDEX-0.8", "RAMINDEX", "KCOV", "NOCOVER", "KCOV-0.0", "NOCOVER-0.0"]
    parser = OptionParser()
    parser.add_option("-B", "--blacklist", dest="blacklist",
                  help="Blacklist", default="")
    parser.add_option("-a", "--all", dest="analyze_all", action="store_true",
                  help="Analyze everything", default=False)
    parser.add_option("-c", "--coverage", dest="analyze_coverage", action="store_true",
                  help="Analyze coverage", default=False)
    parser.add_option("-t", "--triage", dest="analyze_triage", action="store_true",
                  help="Analyze triage", default=False)
    parser.add_option("-w", "--work", dest="analyze_work", action="store_true",
                  help="Analyze work", default=False)
    parser.add_option("-p", "--program", dest="analyze_program", action="store_true",
                  help="Analyze programs", default=False)
    parser.add_option("-m", "--mab", dest="analyze_mab", action="store_true",
                  help="Analyze MAB", default=False)
    parser.add_option("-M", "--mutation-tree", dest="analyze_mutationtree", action="store_true",
                  help="Analyze Mutation Tree", default=False)
    parser.add_option("-s", "--seed", dest="analyze_seed", action="store_true",
                  help="Analyze Seed", default=False)
    parser.add_option("-C", "--crash", dest="analyze_crashes", action="store_true",
                  help="Analyze Crashes", default=False)

    (options, args) = parser.parse_args()
    blacklist = options.blacklist.split(',') if len(options.blacklist) > 0 else []
    tests = [];
    for fn in glob.glob("log_*"):
        skip = False
        for b in blacklist:
            if b in fn:
                skip = True
                break;
        if not skip:
            tests.append(fn.strip("log_"))
    try:
        if options.analyze_coverage or options.analyze_all:
            plotCoverage(tests)
        if options.analyze_triage or options.analyze_all:
            plotTriage(tests)
        if options.analyze_work or options.analyze_all:
            plotWork(tests)
        if options.analyze_mab or options.analyze_all:
            plotMAB(tests)
        if options.analyze_program or options.analyze_all:
            plotPrograms(tests)
        if options.analyze_seed or options.analyze_all:
            plotSeeds(tests)
        if options.analyze_crashes or options.analyze_all:
            plotCrashes(tests)
        if options.analyze_mutationtree or options.analyze_all:
            plotMutationTree(tests)
        #plotSignal(tests)
        #plotCorpus(tests)
        #plotWork(tests)
    except:
        traceback.print_exc()

    """
    data_kcov = parseExp("syzlog_KCOV");
    data_ramindex = parseExp("syzlog_RAMINDEX");
    data_nocover = parseExp("syzlog_NOCOVER");
    # data_random = parseExp("syzlog_RANDOM");
    data = {"Kcov": data_kcov, "Ramindex": data_ramindex, "Nocover": data_nocover};
    plot(data, 1, 2, xlabel="Time elapsed (s)", title="Basic Block Coverage", outfile="execution.png");
    plot(data, 1, 3, xlabel="Time elapsed (s)", title="Hashed Coverage", outfile="execution_hashed.png");
    """
