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

class Program:
    def __init__(self, sig):
        self.id = 0;
        self.ts = 0;
        self.executed = True;
        self.inCorpus = False;
        self.corpusSource = None;
        self.sig = sig
        self.size = 0
        self.coverage = []
        self.coverageCorpus = []
        self.minimize = None
        self.minimizeChildren = []
        self.children = []
        self.origin = None
        self.parent = None
    def __eq__(self, other):
        return self.data == other.data
    def __hash__(self):
        return hash(self.data)
    def _asdict(self):
        return self.__dict__
    @staticmethod
    def FromDict(d):
        ret = Program('')
        for k in d:
            ret.__setattr__(k, d[k])
        return ret

def __processTest(test):
    ret = []
    fn = 'result_' + test
    if not os.path.isfile(fn):
        return set(), [], [];
    p_db = {}
    p_all = [] # No Dedup
    p_generated = []
    p_corpus = {}
    p_triage = {} # Input of triage
    f = open(fn)
    coverageDb = set()
    coverageCorpusDb = set()
    status = "STATUS_NONE"
    status_program = False;
    sig_current = ""
    sig_from = ""
    data_current = ""
    data_from = ""
    p_current = None
    p_from = None
    ts_bgn = 0;
    ts_cur = 0;
    count = {
        "Time_Elapsed": 0,
        "Execute_Count": 0,
        "Generate_Count": 0,
        "Minimize_Count": 0,
        "Mutate_Count": 0,
        "Generate_Signal": 0,
        "Minimize_Signal": 0,
        "Mutate_Signal": 0,
        "Generate_Coverage": 0,
        "Minimize_Coverage": 0,
        "Mutate_Coverage": 0,
    }
    for line in f:
        line = line.strip('\n').strip();
        if len(line) == 0:
            continue;
        if line[:3] == '<<<' and line[-3:] == '>>>':
            ts_cur = int(line.strip('<<<').strip('>>>'))
            if ts_bgn == 0:
                ts_bgn = ts_cur
            count["Time_Elapsed"] = (ts_cur - ts_bgn) / 1000000000.0
            ret.append(copy.deepcopy(count))
        elif line[:2] == '- ' and 'executeRaw' in line:
            # print(status)
            pgsz = int(line.split()[-1])
            if not p_current is None:
                p_current.size = pgsz
            count["Execute_Count"] += 1;
            if "GENERATE" in status:
                count["Generate_Count"] += 1;
            elif "MINIMIZE" in status:
                count["Minimize_Count"] += 1;
            elif "MUTATE" in status:
                count["Mutate_Count"] += 1;
            # ret.append(copy.deepcopy(count))
        elif line[:2] == "+ ":
            try:
                pc = int(line.split()[1], 16)
            except:
                continue
            if (pc & 0xffff000000000000) != 0xffff000000000000 and (pc & 0xffff000000000000) != 0:
                continue
            if not pc in coverageCorpusDb:
                coverageCorpusDb.add(pc)
                if p_current is not None:
                    p_current.coverageCorpus.append(pc)
        elif line[:2] == "= ":
            try:
                pc = int(line.split()[1], 16)
            except:
                continue
            if (pc & 0xffff000000000000) != 0xffff000000000000 and (pc & 0xffff000000000000) != 0:
                continue
            if not pc in coverageDb:
                coverageDb.add(pc)
                if p_current is not None:
                    p_current.coverage.append(pc)
                if "GENERATE" in status:
                    count["Generate_Coverage"] += 1;
                elif "MINIMIZE" in status:
                    count["Minimize_Coverage"] += 1;
                elif "MUTATE" in status:
                    count["Mutate_Coverage"] += 1;
        elif line[0] != "#":
            if (line == '>' or line[:3] == '>>>') and status_program == False:
                status_program = True
                data_current = '';
                sig_current = '';
                if line[:3] == '>>>':
                    sig_current = line.strip('>>>').strip()
                else:
                    data_current += line.strip('>').strip() + '\n'
            elif (line == '<' or line == '<<<') and status_program == True:
                status_program = False
                if status != "MINIMIZE_FROM" and status != "MUTATE_FROM":
                    p_current = Program(sig=sig_current)
                    p_current.ts = (ts_cur - ts_bgn) / 1000000000
                    p_current.id = len(p_all)
                    p_all.append(p_current)
                    if not sig_current in p_db:
                         p_db[sig_current] = p_current.id
                if status == "GENERATE":
                    if p_current.origin is None:
                        p_current.origin = "Generate"
                    #elif p_current.origin != "Generate":
                    #    print("Duplicate program: ", p_current.origin, "Generate")
                    p_generated.append(p_current.id)
                elif status == "MINIMIZE_FROM":
                    sig_from = sig_current
                    if not sig_from in p_triage:
                        p_current = Program(sig=sig_current)
                        p_current.executed = False
                        p_current.ts = (ts_cur - ts_bgn) / 1000000000
                        p_current.id = len(p_all)
                        p_all.append(p_current)
                        if not sig_current in p_db:
                            p_db[sig_current] = p_current.id
                        else:
                            p_current.parent = p_all[p_db[sig_current]].id
                        p_triage[sig_from] = p_current.id
                    p_from = p_all[p_triage[sig_from]]
                elif status == "MINIMIZE_ATTEMPT":
                    # print(p_from.id, p_current.id)
                    #if p_from != p_current:
                    if p_current.origin is None:
                        p_current.origin = "Minimize"
                    p_from.minimizeChildren.append(p_current.id)
                    p_current.parent = p_from.id
                elif status == "MINIMIZE_TO":
                    #if p_from != p_current:
                    if p_current.origin is None:
                        p_current.origin = "Minimize"
                    #elif p_current.origin != "Minimize":
                    #    print("Duplicate program: ", p_current.origin, "Minimize")
                    p_current.executed = False
                    p_from.minimize = p_current.id
                    p_current.parent = p_from.id
                    # print(p_from.id, p_current.id)
                elif status == "MUTATE_FROM":
                    sig_from = sig_current
                    if not sig_from in p_corpus:
                        print("This should not happen!!!!")
                        print(sig_from)
                        p_current = Program(sig=sig_current)
                        p_current.executed = False
                        p_current.ts = (ts_cur - ts_bgn) / 1000000000
                        p_current.id = len(p_all)
                        p_all.append(p_current)
                        if not sig_current in p_db:
                            p_db[sig_current] = p_current.id
                        else:
                            p_current.parent = p_all[p_db[sig_current]].id
                        p_corpus[sig_from] = p_current.id
                    p_from = p_all[p_corpus[sig_from]]
                    status = "MUTATE_TO";
                elif status == "MUTATE_TO":
                    #if p_from != p_current:
                    if p_current.origin is None:
                        p_current.origin = "Mutate"
                        #elif p_current.origin != "Mutate":
                        #    print("Duplicate program: ", p_current.origin, "Mutate")
                    p_from.children.append(p_current.id)
                    p_current.parent = p_from.id
                    # print(p_from.id, p_current.id)
            elif status_program == True:
                if line[:3] != ">>>":
                    data_current += line.strip("> ") + '\n'
        else:
            if "Generate" in line:
                status = "GENERATE"
            elif line[-8:] == "Minimize":
                status = "MINIMIZE_FROM"
            elif "Minimize Attempt" in line:
                status = "MINIMIZE_ATTEMPT"
            elif "Minimize Final" in line:
                status = "MINIMIZE_TO"
            elif "# Result:" in line:
                tmp = line.strip("# Result: ").split(',')
            elif "Mutate" in line:
                status = "MUTATE_FROM"
            elif "addInputToCorpus" in line:
                p_current.inCorpus = True
                crpsrc = int(line.split("Source: ")[1])
                if crpsrc == 0:
                    p_current.corpusSource = "Generate"
                elif crpsrc == 1:
                    p_current.corpusSource = "Mutate"
                elif crpsrc == 2:
                    p_current.corpusSource = "Minimize"
                else:
                    print("WTF")
                sig_current = p_current.sig
                if not sig_current in p_corpus:
                    p_corpus[sig_current] = p_current.id
    f.close();
    return p_all, p_generated, p_corpus, p_triage, ret;

def __addNode(AG, p):
    if p.inCorpus:
        color='red'
        print("Corpus",p.id)
    elif p.origin == "Generate":
        color='blue'
    else:
        color='black'
    AG.add_node(p.id, color=color)
    if p.minimize is None:
        for p_child in p.children:
            print(p.id, p_child.id)
            __addNode(AG, p_child);
            AG.add_edge(p.id, p_child.id)
    else:
        # print(p.id, p.minimize.id)
        __addNode(AG, p.minimize);
        AG.add_edge(p.id, p.minimize.id, color='red')
        for p_child in p.minimizeChildren:
            # print(p.id, p_child.id)
            __addNode(AG, p_child);
            AG.add_edge(p.id, p_child.id, color='green')

def plotProgramGraph(p_generated, outfile="tree.png"):
    AG = PG.AGraph(directed=True, strict=True)
    for p in p_generated:
        __addNode(AG, p)
    AG.layout(prog='dot')
    AG.draw(outfile, format='png', prog='dot')

def plotMutation(p_all, test):
    tmp = {"Mutated Coverage": [], "Corpus Coverage": []}
    tmp_size = {"Mutated Coverage": []}
    corpus_size = [];
    for p_name in p_all:
        p = p_all[p_name]
        if len(p.children) > 0:
            ts = p.ts
            coverage = 0
            for _p in p.children:
                coverage += len(_p.coverage)
            tmp["Mutated Coverage"].append((p.id, ts, coverage))
            tmp_size["Mutated Coverage"].append((p.size, coverage));
        if p.inCorpus:
            ts = p.ts
            coverage = len(p.coverageCorpus)
            tmp["Corpus Coverage"].append((p.id, ts, coverage))
    plotBar(tmp, 1, 2, xlabel="Elapsed time (s)", ylabel="# of coverage", title="", outfile="mutation_%s.png" % test);
    print(tmp_size)
    plot(tmp_size, 0, 1, xlabel="Program Size", ylabel="Coverage", title="", outfile="mutation_size_%s.png" % test, scatter=True);

def __binSplit(data, x, y, c, bin_size=100):
    if len(data) == 0:
        return [], [];
    ret = []
    ret_avg = []
    start = data[0];
    data_bgn = data[0];
    data_prev = data[0];
    for i in range(1, len(data)):
        v = data[i];
        if v[x] - data_bgn[x] >= bin_size:
            sigs = data_prev[y] - data_bgn[y]
            count = data_prev[c] - data_bgn[c]
            ret.append((data_bgn[x], sigs))
            avg = (float(sigs) / count) if count > 0 else 0; 
            ret_avg.append((data_bgn[x], avg))
            data_bgn = v;
        data_prev = v;
    return ret, ret_avg;

def plotPrograms(tests=["KCOV", "RAMINDEX"]):
    datas = {}
    datas_pgsize = {}
    datas_cpsize = {}
    tmax = 0
    cmax = 0
    bin_size = 3600
    datas_jobpower_sum = {}
    datas_seedpower = {}
    datas_seednum = {}
    datas_seedpower_avg = {}
    datas_seedpower_sum = {}
    datas_seedpower_sum_avg = {}
    datas_mutlp = {}
    datas_mutep = {}
    datas_mutls = {}
    for test in tests:
        name, module, run = getTestParams(test)
        name = name + '_' + module
        if not name in datas_seedpower:
            datas_seednum[name] = {
                "Generate": [],
                "Mutate": [],
                "Minimize": [],
                "All": [],
            }
            datas_seedpower[name] = {
                "Generate": [],
                "Mutate": [],
                "Minimize": [],
                "All": [],
            }
            datas_seedpower_avg[name] = {
                "Generate": [],
                "Mutate": [],
                "Minimize": [],
                "All": [],
            }
            datas_seedpower_sum[name] = {
                "Generate": [],
                "Mutate": [],
                "Minimize": [],
            }
            datas_seedpower_sum_avg[name] = {
                "Generate": [],
                "Mutate": [],
                "Minimize": [],
                "All": [],
            }
            datas_jobpower_sum[name] = {
                "Generate": [],
                "Mutate": [],
                "Minimize": [],
            }
            datas_mutls[name] = {
                "Total_Mutations": [],
                "Last_Eff_Mutation": [],
                "Eff_Mutation": []
            }
            datas_mutlp[name] = []
            datas_mutep[name] = []

    for test in tests:
        name, module, run = getTestParams(test)
        name = name + '_' + module
        print("Plotting programs for %s" % test)
        try:
            # p_all, p_generated, p_corpus, p_triage, __data = __processTest(test);
            p_all, p_generated, p_corpus, p_triage, __data = loadDataCached('program_%s.cache', test, __processTest);
            for i in range(len(p_all)):
                if type(p_all[i]) == dict:
                    p_all[i] = Program.FromDict(p_all[i])
        except:
            traceback.print_exc()
            continue;
        #plotMutation(p_all, test)
        print(len(p_all))
        print(len(p_generated))
        print(len(p_corpus))
        print(len(p_triage))
        print(__data[-1])
        datas_jobpower_sum[name]["Generate"].append(__data[-1]["Generate_Coverage"])
        datas_jobpower_sum[name]["Mutate"].append(__data[-1]["Mutate_Coverage"])
        datas_jobpower_sum[name]["Minimize"].append(__data[-1]["Minimize_Coverage"])
        # Accumulated
        datas[test] = {
            "Generation": [(v["Time_Elapsed"], v["Generate_Coverage"]) for v in __data],
            "Minimization": [(v["Time_Elapsed"], v["Minimize_Coverage"]) for v in __data],
            "Mutation": [(v["Time_Elapsed"], v["Mutate_Coverage"]) for v in __data],
        }
        datas[test+"_average"] = {
            "Generation": [(v["Time_Elapsed"], v["Generate_Coverage"] / v["Generate_Count"] if v["Generate_Count"] > 0 else 0) for v in __data],
            "Minimization": [(v["Time_Elapsed"], v["Minimize_Coverage"] / v["Minimize_Count"] if v["Minimize_Count"] > 0 else 0) for v in __data],
            "Mutation": [(v["Time_Elapsed"], v["Mutate_Coverage"] / v["Mutate_Count"] if v["Mutate_Count"] > 0 else 0) for v in __data],
        } 
        plot(datas[test], 0, 1, xlabel="Time elapsed (hr)", ylabel="Total coverage (# edges)", title="", outfile="programs_%s.png" % test, xunit=3600.0);
        plot(datas[test+"_average"], 0, 1, xlabel="Time elapsed (hr)", ylabel="Average coverage (# edges)", title="", outfile="programs_%s_avg.png" % test, ylogscale=True, xunit=3600);
        # By bins
        '''
        data_bin = {
            "Generate_Coverage": __binSplit(__data, x="Time_Elapsed", y="Generate_Coverage", c="Generate_Count", bin_size=bin_size)[0],
            "Minimize_Coverage": __binSplit(__data, x="Time_Elapsed", y="Minimize_Coverage", c="Minimize_Count", bin_size=bin_size)[0],
            "Mutate_Coverage": __binSplit(__data, x="Time_Elapsed", y="Mutate_Coverage", c="Mutate_Count", bin_size=bin_size)[0]
        }
        data_bin_avg = {
            "Generate_Coverage": __binSplit(__data, x="Time_Elapsed", y="Generate_Coverage", c="Generate_Count", bin_size=bin_size)[1],
            "Minimize_Coverage": __binSplit(__data, x="Time_Elapsed", y="Minimize_Coverage", c="Minimize_Count", bin_size=bin_size)[1],
            "Mutate_Coverage": __binSplit(__data, x="Time_Elapsed", y="Mutate_Coverage", c="Mutate_Count", bin_size=bin_size)[1]
        }
        plotBar(data_bin, 0, 1, width=bin_size, xlabel="Time elapsed (hr)", ylabel="# of signals", title="", outfile="programs_bin_%s.png" % test, xunit=3600.0);
        plotBar(data_bin_avg, 0, 1, width=bin_size, xlabel="Time elapsed (hr)", ylabel="# of signals", title="", outfile="programs_bin_avg_%s.png" % test, xunit=3600.0);
        '''
        # Program size
        datas_pgsize[test] = [p.size for p in p_all]
        datas_cpsize[test] = [];
        for psig in p_corpus:
            p = p_all[p_corpus[psig]]
            #if p_all[p].inCorpus:
            datas_cpsize[test].append(p.size)
        # Source of seed
        seed_source = [0,0,0]
        for psig in p_corpus:
            p = p_all[p_corpus[psig]]
            #if p_corpus[p].inCorpus:
            if p.corpusSource == "Generate":
                seed_source[0] += 1
            elif p.corpusSource == "Mutate":
                seed_source[1] += 1
            elif p.corpusSource == "Minimize":
                seed_source[2] += 1
        print(seed_source)
        # Mutation lifespan
        data_ls = {
            "Total_Mutations": [],
            "Last_Eff_Mutation": [],
            "Eff_Mutation": [],
        }
        data_lp = {
            "Lifespan_Percentage": [],
        }
        data_ep = {
            "Eff_Percentage": [],
        }
        for psig in p_corpus:
            p = p_all[p_corpus[psig]]
            #if p_corpus[p].inCorpus:
            tm = len(p.children)
            lm = 0
            em = 0
            for _c in range(len(p.children)):
                c = p_all[p.children[_c]]
                if len(c.coverage) > 0:
                    lm = _c
                    em += 1
            data_ls["Total_Mutations"].append(tm)
            data_ls["Last_Eff_Mutation"].append(lm)
            data_ls["Eff_Mutation"].append(em)
            if tm > 0:
                data_lp["Lifespan_Percentage"].append(float(lm) / float(tm))
                data_ep["Eff_Percentage"].append(float(em) / float(tm))
            else:
                data_lp["Lifespan_Percentage"].append(0.0)
                data_ep["Eff_Percentage"].append(0.0)
        for dn in data_ls:
            datas_mutls[name][dn] += data_ls[dn]
        datas_mutlp[name] += data_lp["Lifespan_Percentage"]
        datas_mutep[name] += data_ep["Eff_Percentage"]
        #plotCDF(data_ls,  xlabel="# Mutations", ylabel="CDF", title="", outfile="mutations_lifespan_%s.png" % test);
        #plotCDF(data_lp,  xlabel="Percentage", ylabel="CDF", title="", outfile="mutations_lifespan_percentage_%s.png" % test);
        # Seed power
        data_seedpower = {
            "Generate": [],
            "Mutate": [],
            "Minimize": [],
            "All": [],
        }
        data_seedpower_avg = {
            "Generate": [],
            "Mutate": [],
            "Minimize": [],
            "All": [],
        }
        data_seedpower_sum = {
            "Generate": 0,
            "Mutate": 0,
            "Minimize": 0
        }
        data_seednum = {
            "Generate": 0,
            "Mutate": 0,
            "Minimize": 0,
            "All": 0
        }
        for psig in p_corpus:
            p = p_all[p_corpus[psig]]
            #if p.inCorpus:
            cov = 0
            origin = p.corpusSource
            if origin is None:
                print("WTF")
                print(psig)
                continue
            data_seednum[origin] += 1
            data_seednum["All"] += 1
            for _c in p.children:
                c = p_all[_c]
                cov += len(c.coverage)
            data_seedpower[origin].append(cov)
            data_seedpower["All"].append(cov)
            data_seedpower_sum[origin] += cov
            if len(p.children) > 0:
                data_seedpower_avg[origin].append(cov / len(p.children))
                data_seedpower_avg["All"].append(cov / len(p.children))
        # print(data_seedpower_avg)
        for job in data_seednum:
            datas_seednum[name][job].append(data_seednum[job])
        for job in data_seedpower:
            datas_seedpower[name][job] += data_seedpower[job]
            datas_seedpower_avg[name][job] += data_seedpower_avg[job]
        for job in data_seedpower_sum:
            datas_seedpower_sum[name][job].append(data_seedpower_sum[job])
            datas_seedpower_sum_avg[name][job].append(data_seedpower_sum[job] / len(data_seedpower[job]) if len(data_seedpower[job]) > 0 else 0.0)
        datas_seedpower_sum_avg[name]["All"].append((data_seedpower_sum["Generate"] + data_seedpower_sum["Mutate"] + data_seedpower_sum["Minimize"]) / len(data_seedpower["All"]) if len(data_seedpower["All"]) > 0 else 0.0) 
    tmp = {}
    for name in datas_seedpower:
        plotCDF(datas_mutls[name],  xlabel="# Mutations", ylabel="CDF", title="", outfile="mutations_lifespan_%s.png" % name, xrange=(-25, 825), small=True);
        plotCDF(datas_seedpower[name], xlabel="Coverage", ylabel="CDF", title="", outfile="seed_power_%s.png" % name, xrange=(-0.5, 1005), xlogscale=True, small=True);
        plotCDF(datas_seedpower_avg[name], xlabel="Coverage", ylabel="CDF", title="", outfile="seed_power_avg_%s.png" % name, xrange=(-0.5,10), xlogscale=False, small=True);
        tmp[name] = datas_seedpower[name]["All"]
    plotCDF(tmp, xlabel="Coverage", ylabel="CDF", title="", outfile="seed_power_all.png", xrange=(-0.5, 1005), xlogscale=True);
    # Seed power sum
    #for name in datas_seedpower_sum:
    #    for job in datas_seedpower_sum[name]:
    #        datas_seedpower_sum[name][job] = np.median(datas_seedpower_sum[name][job])
    plotBar1(datas_seednum, ylabel="# seeds", outfile="seed_num.png")
    plotBar1(datas_seedpower_sum, ylabel="Coverage", outfile="seed_power_sum.png")
    plotBar1(datas_seedpower_sum_avg, ylabel="Coverage", outfile="seed_power_sum_avg.png")
    plotBar1(datas_jobpower_sum, ylabel="Coverage", outfile="work_power_sum.png")

    plotCDF(datas_mutlp,  xlabel="Percentage", ylabel="CDF", title="", outfile="mutations_lifespan_percentage.png", xrange=(-0.05, 1.05));
    plotCDF(datas_mutep,  xlabel="Percentage", ylabel="CDF", title="", outfile="mutations_eff_percentage.png", xrange=(-0.05, 1.05));

    plotCDF(datas_pgsize, xlabel="Program Size", ylabel="CDF", title="", outfile="programs_size.png");
    plotCDF(datas_cpsize, xlabel="Corpus Size", ylabel="CDF", title="", outfile="corpus_size.png");

