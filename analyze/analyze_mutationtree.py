import sys
import os
import copy
import traceback
import simplejson as json
import random
import math
import matplotlib.pyplot as plt
from matplotlib.collections import PatchCollection
from matplotlib.patches import Rectangle
import numpy as np
import pygraphviz as PG
# from igraph import Graph, EdgeSeq

from plot import plot, plotBar, plotCDF, plotBar1, plotCDF2
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
        self.id = None
    def computeLevel(self, level=-1, visited=None, maxlevel=10000000):
        if level >= 0:
            self.level = level
        height = self.level
        self.size = 1
        if self.level >= maxlevel:
            self.children = []
        for n in self.children:
            if visited is not None and n.program.sig in visited:
                print("Cycle detected:", n.program.sig)
                continue
            n.computeLevel(self.level + 1, visited, maxlevel)
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
    def getOffspring(self):
        ret = [self]
        for n in self.children:
            ret += n.getOffspring()
        return ret
        
    #def __eq__(self, other):
    #    return self.p.sig == self.other.sig

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
        # Ignore minimize nodes
        if p.origin == "Minimize":
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
            dist = 1
            while p_parent.origin == "Minimize" and p_parent.parent is not None: # Ignore minimize edges
                p_parent = p_all[p_parent.parent]
                dist += 1
            #print(p_parent.sig, p_cur.sig, dist)
            #exit()
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

def sortSample(l, n):
    if n == 0:
        return []
    tmp = sorted(l, reverse=True, key=lambda x: x.size)
    step = len(l) / float(n)
    ret = []
    for i in range(n):
        idx = round(step * (i))
        if 0 <= idx < len(tmp):
            ret.append(tmp[idx])
        else:
            ret.append(tmp[-1])
    return ret

def sampleTreeNoAdd(trees, nodes, samplerate=0.05):
    # Node sampling
    nodes_sample = random.sample(nodes.keys(), math.ceil(len(nodes) * samplerate))
    # Reconstruct sampled tree via back-tracking 
    nodes_sample_dedup = {}
    # Add all nodes to nodes
    for n in nodes_sample:
        nodes_sample_dedup[nodes[n].program.sig] = None
    trees_sample = []
    for nsig in nodes_sample:
        #if nsig in nodes_sample_dedup:
        #    continue
        n_cur = nodes[nsig]
        n_new_cur = Node(nodes[nsig].program)
        nodes_sample_dedup[nsig] = n_new_cur
        n_parent = n_cur.parent
        root_reached = True
        while n_parent is not None:
            n_parent_sig = n_parent.program.sig
            if n_parent_sig in nodes_sample_dedup: # This check ensures there will be no cycles
                print(n_parent_sig,'->',n_cur.program.sig)
                if nodes_sample_dedup[n_parent_sig] is None:
                    nodes_sample_dedup[n_parent_sig] = Node(n_parent.parent)
                    n_new_parent = nodes_sample_dedup[n_parent_sig]
                    n_new_cur.parent = n_new_parent
                    n_new_parent.children.append(n_new_cur)
                else:
                    n_new_parent = nodes_sample_dedup[n_parent_sig]
                    n_new_cur.parent = n_new_parent
                    n_new_parent.children.append(n_new_cur)
                    root_reached = False
                    break
            else:
                n_cur = n_parent
                n_parent = n_cur.parent
        print(root_reached)
        if root_reached:
            #if len(n_new_cur.children) > 0:
                trees_sample.append(n_new_cur)
    for n in trees_sample:
        nodes_visited = set() # Check whether there are cycles
        n.computeLevel(visited=nodes_visited)
    print(trees_sample, len(nodes_sample_dedup))
    return trees_sample, nodes_sample_dedup


def sampleTree(trees, nodes, samplerate=0.05, maxlevel=100):
    # Node sampling
    nodes_sample = random.sample(nodes.keys(), math.ceil(len(nodes) * samplerate))
    # Reconstruct sampled tree via back-tracking 
    nodes_sample_dedup = {}
    trees_sample = []
    for nsig in nodes_sample:
        if nsig in nodes_sample_dedup:
            continue
        n_cur = nodes[nsig]
        n_new_cur = Node(nodes[nsig].program)
        nodes_sample_dedup[nsig] = n_new_cur
        n_parent = n_cur.parent
        root_reached = True
        while n_parent is not None:
            n_parent_sig = n_parent.program.sig
            if n_parent_sig in nodes_sample_dedup: # This check ensures there will be no cycles
                n_new_parent = nodes_sample_dedup[n_parent_sig]
                n_new_cur.parent = n_new_parent
                n_new_parent.children.append(n_new_cur)
                root_reached = False
                break
            else:
                n_new_parent = Node(n_parent.program)
                nodes_sample_dedup[n_parent_sig] = n_new_parent
                n_new_cur.parent = n_new_parent
                n_new_parent.children.append(n_new_cur)
                n_new_cur = n_new_parent
                n_cur = n_parent
                n_parent = n_cur.parent
        if root_reached and n_new_cur.program.origin == "Generate":
            if len(n_new_cur.children) > 0:
                trees_sample.append(n_new_cur)
    for n in trees_sample:
        nodes_visited = set() # Check whether there are cycles
        n.computeLevel(visited=nodes_visited, maxlevel=maxlevel)
    print(trees_sample, len(nodes_sample_dedup))
    return trees_sample, nodes_sample_dedup

def plotForest(trees, nodes, samplerate=1.0, prunerate=0.0, maxnodes=500, maxtrees=100000, maxlevel=5000, outfile="tree.png"):
    #if maxnodes / len(nodes) < samplerate / 2:
    samplerate = maxnodes * 2.0 / len(nodes)
    AG = PG.AGraph(directed=True, strict=True)
    AG.graph_attr.update(penwidth=3, ratio='fill', size='10,16')
    AG.node_attr.update(color='black', shape='circle', style='filled', fillcolor='red',penwidth=3)
    AG.edge_attr.update(len='2.0',penwidth=3)
    if samplerate < 1.0:
        trees_sample, nodes_sample_dedup = sampleTree(trees, nodes, samplerate=samplerate, maxlevel=maxlevel)
    else:
        trees_sample = trees
    # Prune randomly if necessary
    size_total = 0
    for r in trees_sample:
        size_total += r.size
    print(size_total)
    while size_total > maxnodes:
        nsig = random.choice(list(nodes_sample_dedup.keys()))
        n = nodes_sample_dedup[nsig]
        # Remove 
        if n.parent is not None and n in n.parent.children:
            n.parent.children.remove(n)
        if n in trees_sample:
            trees_sample.remove(n)
        # Recompute sizes and everything
        size_total = 0
        for r in trees_sample:
            nodes_visited = set() # Check whether there are cycles
            r.computeLevel(visited=nodes_visited, maxlevel=maxlevel)
            size_total += r.size
        # Remove nodes from pool
        deleted_nodes = n.getOffspring()
        print("Deleting:", n, n.size)
        for dn in deleted_nodes:
            dnsig = dn.program.sig
            if dnsig in nodes_sample_dedup:
                del nodes_sample_dedup[dnsig]
        #for n in trees_sample:
        #    nodes_visited = set() # Check whether there are cycles
        #    n.computeLevel(visited=nodes_visited, maxlevel=maxlevel)
    print(size_total)
    '''
    if len(trees_sample) > 0:
        n_trees = min(math.ceil(len(trees_sample) * (1.0 - prunerate)), maxtrees)
        # n_trees = 1 + round(math.log(len(trees_sample)))
        n_trees = math.ceil(math.pow(len(trees_sample), 0.33))
        trees_sample = sortSample(trees_sample, n_trees)
    '''
    # Plot trees, prune along the way
    queue = []
    nid = 0
    for root in trees_sample:
        if len(root.children) == 0: # Ignore stray nodes
            continue
        root.level = 1
        queue.append(root)
        root.id = nid
        nid += 1
    cur_level = 1
    while len(queue) > 0 and maxnodes > 0:
        # print([x.id for x in queue])
        node = queue.pop(0)
        # Check if we need pruning
        #if len(queue) > 0 and queue[0].level > cur_level:
        #    n_nodes = math.ceil(len(queue) * (1.0 - prunerate))
        #    queue = sortSample(queue, n_nodes)
        AG.add_node(node.id, label='')
        maxnodes -= 1
        if node.level < maxlevel and len(node.children) > 0:
          n_childs = math.ceil(len(node.children) * (1.0 - prunerate))
          # n_childs = 1 + round(math.log(len(node.children)))
          # n_childs = math.ceil(math.pow(len(node.children), 0.33))
          children_subset = sortSample(node.children, n_childs)
          for c in children_subset:
            queue.append(c)
            c.id = nid
            c.level = node.level + 1
            c.parent = node
            nid += 1
        # Edge to parent
        if node.parent is not None:
            parent_id = node.parent.id
            if parent_id is None:
                print("WTF")
            AG.add_edge(parent_id, node.id)
            print(parent_id, node.id)

    '''
    n_trees = math.ceil(len(trees) * samplerate)
    print(n_trees)
    trees_subset = sortSample(trees, n_trees)
    print(trees_subset)
    # Construct tree in BFS fashion
    queue = []
    nid = 0
    for root in trees_subset:
        queue.append(root)
        root.id = nid
        nid += 1
    while len(queue) > 0 and maxnodes > 0:
        node = queue.pop(0)
        AG.add_node(nid)
        maxnodes -= 1
        # Child sampling
        n_childs = math.ceil(len(node.children) * samplerate)
        children_subset = sortSample(node.children, n_childs)
        # Add children to queue
        for c in children_subset:
            queue.append(c)
            c.id = nid
            nid += 1
        # Edge to parent
        if node.parent is not None:
            parent_id = node.parent.id
            if parent_id is None:
                print("WTF")
            AG.add_edge(parent_id, node.id)
            print(parent_id, node.id)
    '''
    # Draw tree
    AG.layout(prog='dot')
    AG.draw(outfile, format='png', prog='dot')
    AG.draw(outfile + ".pdf", format='pdf', prog='dot')

def plotMutationTree(tests):
    datas = {}
    for test in tests:
        name, module, run = getTestParams(test)
        # name = name + '_' + module
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
        # name = name + '_' + module
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
        plotForest(trees, nodes, outfile="mt_sample_%s.png" % test)
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
    # plotCDF(datas, key="Tree_Height", xlabel="Tree Height", ylabel="Cumulative # Trees", title="", outfile="mt_height_overall.png", xlogscale=True, raw=True, small=False);
    # plotCDF(datas, key="Tree_Size", xlabel="Tree Size", ylabel="Cumulative # Trees", title="", outfile="mt_size_overall.png", xlogscale=True, raw=True, small=False);
    plotCDF2([datas, datas], key=["Tree_Size", "Tree_Height"], xlabel=["Tree Size", "Tree Height"], ylabel=["Cumulative # Trees", "Cumulative # Trees"], outfile="mt_size_height_overall.png", xlogscale=[True, True], raw=[True, True]);
    # plotCDF(datas, key="Node_Degree", xlabel="Node Degree", ylabel="Cumulative # Nodes", title="", outfile="mt_degree_overall.png", xlogscale=True, raw=True);

    #plotCDF(datas, key="Tree_Height", xlabel="Tree Height", ylabel="CDF", title="", outfile="mt_height_overall_cdf.png", xlogscale=False, xrange=(-5,105));
    #plotCDF(datas, key="Tree_Size", xlabel="Tree Size", ylabel="CDF", title="", outfile="mt_size_overall_cdf.png", xlogscale=True);
    #plotCDF(datas, key="Node_Degree", xlabel="Tree Size", ylabel="CDF", title="", outfile="mt_degree_overall_cdf.png", xlogscale=True);

    # Leaf nodes per tree
    #plotCDF(datas, key="Leaf_Nodes_Per_Tree", xlabel="# of nodes", ylabel="CDF", title="", outfile="mt_leaf_per_tree_cdf.png", xlogscale=True);
    #plotCDF(datas, key="Leaf_Nodes_Percentage_Per_Tree", xlabel="# of nodes", ylabel="CDF", title="", outfile="mt_leaf_per_tree_perc_cdf.png", xlogscale=False);

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

    

