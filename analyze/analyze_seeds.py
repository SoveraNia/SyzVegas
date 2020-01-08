import sys
import os
import copy
import traceback
import simplejson as json
import matplotlib.pyplot as plt
from matplotlib.collections import PatchCollection
from matplotlib.patches import Rectangle
import numpy as np
import pygraphviz as PG

from plot import plot, plotBar, plotCDF, plotBar1
from utils import loadDataCached, getTestParams
from analyze_programs import Program, __processTest 


def analyzeExclusiveCoverage(data, names, coverage_all, coverage):
    # First, find all exlucsive coverages
    cov_exclusive = [[], []]
    for c in coverage_all:
        if c in coverage[0] and not c in coverage[1]:
            cov_exclusive[0].append(c)
        elif c in coverage[1] and not c in coverage[0]:
            cov_exclusive[1].append(c)
    print(len(cov_exclusive[0]), len(cov_exclusive[1]))
    tmp = {"Exclusive Coverage": {}}
    for tidx in range(2):
        tmp["Exclusive Coverage"][names[tidx]] = [len(cov_exclusive[tidx])]
    plotBar1(tmp, ylabel="Coverage", outfile="exclusive_cov_sum_%s_vs_%s.png" % (names[0], names[1]))
    # Next, find all programs
    # We also try to find the very source of the program
    p_exclusive = [[], []]
    source_time = {}
    chain_length = {}
    for tidx in range(2):
      name = names[tidx]
      source_time[name] = []
      chain_length[name] = []
      for r in range(len(data[tidx])):
        p_all = data[tidx][r][0]
        for p in p_all:
          if not p.executed:
            continue
          cov = 0
          for c in p.coverage:
            if c in cov_exclusive[tidx]:
              cov += 1
          if cov > 0:
            p_exclusive[tidx].append((p, cov))
            # Backtrack
            p_cur = p
            l = 0
            while p_cur.parent is not None:
                if p_cur.data != p_cur.parent.data:
                    l += 1
                p_cur = p_cur.parent
            if p_cur.origin != "Generate":
                continue
            # We ignore the initial generated program
            if p_cur.id == 0:
                continue
            for i in range(cov):
                source_time[name].append(p_cur.ts)
                chain_length[name].append(l)
            print(p_cur.data, p_cur.id, p_cur.ts, l)
    print(len(p_exclusive[0]), len(p_exclusive[1]))
    plotCDF(source_time, xlabel="Time (s)", ylabel="CDF", title="", outfile="exclusive_cov_source_time_%s_vs_%s.png" % (names[0], names[1])); 
    plotCDF(chain_length, xlabel="# of Mutations / Minimizations", ylabel="CDF", title="", outfile="exclusive_cov_chain_length_%s_vs_%s.png" % (names[0], names[1]));
    return


    # Finally, breakdown where the programs come from
    print("Exclusive Coverage")
    for tidx in range(2):
      print("Test",tidx)
      origin_exclusive = {"Generate": 0, "Mutate": 0, "Minimize": 0}
      corpusorigin_exclusive = {"Generate": 0, "Mutate": 0, "Minimize": 0}
      for p, cov in p_exclusive[tidx]:
          if p.origin is not None:
              origin_exclusive[p.origin] += cov
          if p.origin == "Mutate":
              if p.parent is None:
                  continue
              corpus_origin = p.parent.corpusSource
              if corpus_origin is not None:
                  corpusorigin_exclusive[p.origin] += cov
      print(origin_exclusive)
      print(corpusorigin_exclusive)
    

def __plotSeeds(tests, names):
    datas = [{}, {}]
    name0 = names[0]
    name1 = names[1]

    coverageCorpus_all = {}
    coverageCorpus = [{}, {}]
    coverage_all = {}
    coverage = [{}, {}]
    seeds = [[], []]
    coverages_exclusive = [set(), set()]
    
    data = [[], []]

    # All corpus coverage
    for test in tests:
        name, module, run = getTestParams(test)
        name = name + '_' + module
        if name0 in name or name1 in name:
            print(test)
            tidx = 0
            if name1 in name:
                tidx = 1
            d = __processTest(test)
            data[tidx].append(d)
            p_all, p_generated, p_corpus, p_triage, __data = d
            datas[tidx][test] = (p_all, p_generated)
            for p in p_all:
                # All coverage
                for c in p.coverage:
                    if not c in coverage_all:
                         coverage_all[c] = 0
                    if not c in coverage[tidx]:
                         coverage[tidx][c] = 0
                    coverage_all[c] += 1
                    coverage[tidx][c] += 1
            for pname in p_corpus:
                p = p_corpus[pname]
                # Corpus Coverage
                for c in p.coverageCorpus:
                    if not c in coverageCorpus_all:
                         coverageCorpus_all[c] = 0
                    if not c in coverageCorpus[tidx]:
                         coverageCorpus[tidx][c] = 0
                    coverageCorpus_all[c] += 1
                    coverageCorpus[tidx][c] += 1
                # Seed power
                cov = 0
                for c in p.children:
                    cov += len(c.coverage)
                seeds[tidx].append((p, cov))
    print(len(coverage[0]), len(coverage[1]))
    print(len(coverageCorpus[0]), len(coverageCorpus[1]))

    analyzeExclusiveCoverage(data, names, coverage_all, coverage)
    return

    # Seed power
    # Look for useless seeds
    useless_seeds = [[], []]
    for p_sig, p, cov in seeds[0]:
        if cov == 0:
            useless_seeds[0].append(p)
    for p_sig, p, cov in seeds[1]:
        if cov == 0:
            useless_seeds[1].append(p)
    print("Useless seeds")
    print(len(useless_seeds[0]), len(seeds[0]), len(useless_seeds[0]) / len(seeds[0]))
    print(len(useless_seeds[1]), len(seeds[1]), len(useless_seeds[1]) / len(seeds[1]))

    # Coverage for useless seeds
    useless_coverage = [
        {"Exclusive_0": {}, "Exclusive_1": {}},
        {"Exclusive_0": {}, "Exclusive_1": {}}
    ]
    for tidx in range(2):
        s0 = 0
        s1 = 0
        for s in useless_seeds[tidx]:
            for c in s.coverageCorpus:
                if c in coverages[0]:
                    if not c in useless_coverage[tidx]["Exclusive_0"]:
                        useless_coverage[tidx]["Exclusive_0"][c] = 0
                    useless_coverage[tidx]["Exclusive_0"][c] += coverages[0][c]
                    s0 += coverages[0][c]
                if c in coverages[1]:
                    if not c in useless_coverage[tidx]["Exclusive_1"]:
                        useless_coverage[tidx]["Exclusive_1"][c] = 0
                    useless_coverage[tidx]["Exclusive_1"][c] += coverages[1][c]
                    s1 += coverages[1][c]
        print("----")
        print(len(useless_coverage[tidx]["Exclusive_0"]))
        print(len(useless_coverage[tidx]["Exclusive_1"]))
        print(s0, s1)

def plotSeeds(tests):
    names = [
        ["Default-SS", "MAB-NaelIX3.1-SS"],
        #["Default-SS", "Default-Sync"],
    ]
    for n in names:
        __plotSeeds(tests, n)
        
