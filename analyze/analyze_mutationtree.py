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

class Node:
    def __init__(self, p: Program):
        self.program = p
        self.level = -1
        self.height = -1 # Starting from this node, the max height
        self.size = 1
        self.children = []
        self.parent = None
    def computeLevel(self, level=-1, visited=None):
        if level >= 0:
            self.level = level
        height = self.level
        self.size = 1
        for n in self.children:
            if visited is not None and n.program.sig in visited:
                print("Cycle detected:", n.program.sig)
                continue
            n.computeLevel(self.level + 1, visited)
            d = n.height
            s = n.size
            self.size += s
            height = d if d > height else height
        self.height = height
        return height
    def collectDegrees(self, result, visited=None):
        degree = 0
        leaves = 0
        for n in self.children:
            if visited is not None and n.program.sig in visited:
                print("Cycle detected:", n.program.sig)
                continue
            degree += 1
            l = n.collectDegrees(result, visited=visited)
            leaves += l
        if degree > 0: # Ignore leaf nodes
            result.append(degree)
            return leaves
        else:
            return 1
        
    def __eq__(self, other):
        return self.p.sig == self.other.sig

def buildMutationTrees(p_all, p_generated, p_corpus, p_triage):
    roots = []
    nodes_all_dedup = {}
    # Establish roots
    for pid in p_generated:
        p = p_all[pid]
        if not p.sig in nodes_all_dedup:
            n = Node(p)
            n.level = 0
            nodes_all_dedup[p.sig] = n 
            roots.append(n)
    # Back trace to build the tree
    for i in range(len(p_all)):
        p = p_all[i]
        if not p.executed:
            continue
        if p.sig in nodes_all_dedup:
            continue
        # Backtrack
        p_cur = p
        n_cur = Node(p)
        nodes_all_dedup[p.sig] = n_cur
        n_parent = None
        p_parent = None
        root_reached = True
        while p_cur.parent is not None:
            p_parent = p_all[p_cur.parent]
            if p_parent.sig in nodes_all_dedup: # This check ensures there will be no cycles
                n_parent = nodes_all_dedup[p_parent.sig]
                n_cur.parent = n_parent
                n_parent.children.append(n_cur)
                root_reached = False
                break
            else:
                n_parent = Node(p_parent)
                nodes_all_dedup[p_parent.sig] = n_parent
                n_cur.parent = n_parent
                n_parent.children.append(n_cur)
                p_cur = p_parent
                n_cur = n_parent
    # Sort out degree from root
    for n in roots:
        nodes_visited = set() # Check whether there are cycles
        n.computeLevel(visited=nodes_visited)
        # print(n.program.sig, n.height, len(n.children), len(n.program.children), len(n.program.minimizeChildren))
    __roots = []
    # Remove isolated roots
    for n in roots:
        if len(n.children) > 0:
            __roots.append(n)
    return __roots, nodes_all_dedup

def plotMutationTree(tests):
    datas = {}
    for test in tests:
        name, module, run = getTestParams(test)
        name = name + '_' + module
        datas[name] = {
            "Num_Trees": [],
            "Tree_Height": [],
            "Node_Degree": [],
            "Leaf_Nodes": [],
            "Leaf_Nodes_Percentage": [],
            "Leaf_Nodes_Per_Tree": [],
            "Leaf_Nodes_Percentage_Per_Tree": [],
            "Tree_Size": [],
            "Seed_Subtree_Height": {"Generate": [], "Mutate": [], "Minimize": [], "All": []},
            "Seed_Subtree_Size": {"Generate": [], "Mutate": [], "Minimize": [], "All": []}
        }
    for test in tests:
        name, module, run = getTestParams(test)
        name = name + '_' + module
        print("Plotting mutation tree for %s" % test)
        try:
            p_all, p_generated, p_corpus, p_triage, __data = loadDataCached('program_%s.cache', test, __processTest);
            for i in range(len(p_all)):
                if type(p_all[i]) == dict:
                    p_all[i] = Program.FromDict(p_all[i])
        except:
            traceback.print_exc()
            continue;
        trees, nodes = buildMutationTrees(p_all, p_generated, p_corpus, p_triage)
        print(len(trees), len(nodes))
        datas[name]["Num_Trees"].append(len(trees))
        num_lf = 0
        for r in trees:
            # For generation, just consider ones that're mutated
            if r.height > 0 and r.size > 1:
                datas[name]["Tree_Height"].append(r.height)
                datas[name]["Tree_Size"].append(r.size)
            degrees = []
            visited = set()
            l = r.collectDegrees(degrees, visited)
            # print(l, len(degrees))
            datas[name]["Node_Degree"] += degrees
            num_lf += l
            datas[name]["Leaf_Nodes_Per_Tree"].append(l)
            datas[name]["Leaf_Nodes_Percentage_Per_Tree"].append(l / r.size)
        datas[name]["Leaf_Nodes"].append(num_lf)
        if len(nodes) > 0:
            datas[name]["Leaf_Nodes_Percentage"].append(100.0 * num_lf / len(nodes))
        for sig in nodes:
            n = nodes[sig]
            # datas[name]["Node_Degree"].append(len(n.children))
            csig = n.program.sig
            if csig in p_corpus:
                src = p_all[p_corpus[csig]].corpusSource
                if src is not None:
                    datas[name]["Seed_Subtree_Height"][src].append(n.height)
                    datas[name]["Seed_Subtree_Size"][src].append(n.size)
                    datas[name]["Seed_Subtree_Height"]["All"].append(n.height)
                    datas[name]["Seed_Subtree_Size"]["All"].append(n.size)
    # Overall Tree size and height
    plotCDF(datas, key="Tree_Height", xlabel="Max Tree Height", ylabel="CD", title="", outfile="mt_height_overall.png", xlogscale=False, raw=True, xrange=(-5,105));
    plotCDF(datas, key="Tree_Size", xlabel="Tree Size", ylabel="CD", title="", outfile="mt_size_overall.png", xlogscale=True, raw=True);
    plotCDF(datas, key="Node_Degree", xlabel="Tree Size", ylabel="CD", title="", outfile="mt_degree_overall.png", xlogscale=True, raw=True);

    plotCDF(datas, key="Tree_Height", xlabel="Max Tree Height", ylabel="CDF", title="", outfile="mt_height_overall_cdf.png", xlogscale=False, xrange=(-5,105));
    plotCDF(datas, key="Tree_Size", xlabel="Tree Size", ylabel="CDF", title="", outfile="mt_size_overall_cdf.png", xlogscale=True);
    plotCDF(datas, key="Node_Degree", xlabel="Tree Size", ylabel="CDF", title="", outfile="mt_degree_overall_cdf.png", xlogscale=True);

    # Leaf nodes per tree
    plotCDF(datas, key="Leaf_Nodes_Per_Tree", xlabel="# of nodes", ylabel="CDF", title="", outfile="mt_leaf_per_tree_cdf.png", xlogscale=True);
    plotCDF(datas, key="Leaf_Nodes_Percentage_Per_Tree", xlabel="# of nodes", ylabel="CDF", title="", outfile="mt_leaf_per_tree_perc_cdf.png", xlogscale=False);

    # Num Trees
    tmp = {}
    for name in datas:
        #tmp["Num_Trees"][name] = datas[name]["Num_Trees"]
        tmp[name] = {"Num_Trees": datas[name]["Num_Trees"]}
    plotBar1(tmp, ylabel="# Nodes", outfile="mt_num_trees.png")


    # Leaf nodes total
    tmp = {}
    for name in datas:
        #tmp["Leaf_Nodes"][name] = datas[name]["Leaf_Nodes"]
        tmp[name] = {"Leaf_Nodes": datas[name]["Leaf_Nodes"]}
    plotBar1(tmp, ylabel="# Nodes", outfile="mt_leaf_per_forest.png")
    tmp = {"Leaf_Nodes_Percentage": {}}
    for name in datas:
        #tmp["Leaf_Nodes_Percentage"][name] = datas[name]["Leaf_Nodes_Percentage"]
        tmp[name] = {"Leaf_Nodes_Percentage": datas[name]["Leaf_Nodes_Percentage"]}
    plotBar1(tmp, ylabel="Percentage (%)", outfile="mt_leaf_per_forest_perc.png")

    # Sub-tree size, height
    tmp = {"Generate": {}, "Mutate": {}, "Minimize": {}, "All": {}}
    for src in ["Generate", "Mutate", "Minimize", "All"]:
        for name in datas:
            tmp[src][name] = datas[name]["Seed_Subtree_Height"][src]
        plotCDF(tmp[src], xlabel="Tree height", ylabel="CDF", title="", outfile="mt_subtree_height_cdf_%s.png" % src, xlogscale=False);
    tmp = {"Generate": {}, "Mutate": {}, "Minimize": {}, "All": {}}
    for src in ["Generate", "Mutate", "Minimize", "All"]:
        for name in datas:
            tmp[src][name] = datas[name]["Seed_Subtree_Size"][src]
        plotCDF(tmp[src], xlabel="Tree height", ylabel="CDF", title="", outfile="mt_subtree_size_cdf_%s.png" % src, xlogscale=True);

    

