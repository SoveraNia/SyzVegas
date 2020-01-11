import matplotlib.pyplot as plt
from matplotlib.collections import PatchCollection
from matplotlib.patches import Rectangle
import numpy as np

linecolors = ["r", "g", "b", "black"]
markers = ['s', 'o', '^', 'v', '+', 'x', 'd']
fillcolors = ["tab:red", "tab:olive", "tab:blue", "tab:cyan"]
patterns = ['--', 'xx', '++', '\\\\', '**', '..']
linestyles = ["-",
     (0, (1, 3)),
     (0, (5, 5)),
     (0, (3, 5, 1, 5)),
     (0, (3, 5, 1, 5, 1, 5)),

     (0, (1, 1)),
     (0, (5, 8)),
     (0, (3, 1, 1, 1)),
     (0, (3, 10, 1, 10, 1, 10)),
     
     (0, (5, 1)),
     (0, (3, 10, 1, 10)),
     (0, (3, 1, 1, 1, 1, 1))]

order = [""]
def sortKeys(keys):
    prefix = []
    mid = []
    postfix = []
    for k in keys:
        if '+' in k:
            prefix.append(k)
        elif 'Default' in k:
            postfix.insert(0, k)
        else:
            mid.append(k)
    mid.sort()
    return prefix+mid+postfix

def plot(data, key, value, xlabel="", ylabel="", title="", outfile="out.png",
xlogscale=False, ylogscale=False, xmax=None, ymax=None, scatter=False, xunit=1.0, yunit=1.0, nmarkers=12, xstep=None, small=False):
    fig = None
    ax = None
    if small:
        fig = plt.figure(figsize=(4,4))
        ax = plt.subplot(111)
        plt.subplots_adjust(left=0.2, bottom=0.2, right=0.95, top=0.98, wspace=0, hspace=0)
    else:
        fig = plt.figure(figsize=(8,5))
        ax = plt.subplot(111)
        plt.subplots_adjust(left=0.15, bottom=0.15, right=0.95, top=0.95, wspace=0, hspace=0)

    ax.set_title(title, fontsize=20);
    ax.set_xlabel(xlabel, fontsize=16);
    ax.set_ylabel(ylabel, fontsize=16);
    ax.tick_params(labelsize=12);
    if not xmax is None:
        ax.set_xlim(0,xmax);
    if not ymax is None:
        ax.set_ylim(0,ymax);
    if xlogscale:
        ax.set_xscale('symlog')
    if ylogscale:
        ax.set_yscale('symlog')
    idx = 0;
    maxx = 0
    for test in sortKeys(data.keys()):
        label = test.replace("KCOV", "").replace('_', ' ').strip()
        if len(data[test]) == 0:
            continue;
        x = [v[key] / xunit for v in data[test]];
        y = [v[value] / yunit for v in data[test]];
        maxx = maxx if maxx > x[-1] else x[-1]
        marker = markers[int(idx%len(markers))] if nmarkers > 1 else None
        markevery = int((len(x)-1) / (nmarkers-1)) if nmarkers > 1 else None
        if markevery == 0:
            markevery = 1
        if not scatter:
            ax.plot(x,y, label=label, color=linecolors[idx%len(linecolors)], linestyle=linestyles[int(idx/len(linecolors))], marker=marker, markersize=8, markevery=markevery);
        else:
            ax.scatter(x,y, label=label, color=linecolors[idx%len(linecolors)], marker=marker, markersize=8, markevery=markevery);
        idx += 1;
    if xstep is not None:
        ax.set_xticks([xstep*i for i in range(int(round(maxx / xstep))+1)])
    ax.grid();
    # ax.legend(bbox_to_anchor=(1.01, 1.0))
    ax.legend(loc=0, fontsize=12)
    plt.savefig(outfile); 
    plt.savefig(outfile + '.pdf');
    plt.close('all');

def plotBar1(data, width=1, xlabel="", ylabel="", title="", outfile="out.png",
xlogscale=False, ylogscale=False, xmax=None, ymax=None, small=False):
    fig = None
    ax = None
    total_test_len = 0
    for test in data.keys():
        total_test_len += len(test)
    bbox_to_anchor = False
    if len(data.keys()) > 4 or total_test_len > 40:
        bbox_to_anchor = True
    if small:
        fig = plt.figure(figsize=(4,4))
        ax = plt.subplot(111)
        plt.subplots_adjust(left=0.2, bottom=0.2, right=0.95, top=0.98, wspace=0, hspace=0)
    else:
        fig = plt.figure(figsize=(8,5))
        ax = plt.subplot(111)
        if bbox_to_anchor:
            plt.subplots_adjust(left=0.15, bottom=0.15, right=0.7, top=0.9, wspace=0, hspace=0)
        else:
            plt.subplots_adjust(left=0.15, bottom=0.15, right=0.95, top=0.9, wspace=0, hspace=0)
    ax.set_title(title, fontsize=20);
    ax.set_xlabel(xlabel, fontsize=16);
    ax.set_ylabel(ylabel, fontsize=16);
    ax.tick_params(labelsize=12);

    if not xmax is None:
        ax.set_xlim(0,xmax);
    if not ymax is None:
        ax.set_ylim(0,ymax);
    if xlogscale:
        ax.set_xscale('symlog')
    if ylogscale:
        ax.set_yscale('symlog')
    idx = 0;
    bar_width = float(width) / (len(data.keys()) + 1)
    labels = None
    for test in sortKeys(data.keys()):
        if len(data[test]) == 0:
            continue;
        if labels is None:
            labels = sorted(data[test].keys())
        x = []
        y_mean = []
        y_std = []
        for key in labels:
            x.append(len(x) * width + (idx+0.5)*bar_width);
            y_mean.append(np.mean(data[test][key]))
            y_std.append(np.std(data[test][key]))
        tlabel = test.replace("KCOV", "").replace('_', ' ').strip()
        ax.bar(x,y_mean, yerr=y_std, width=bar_width, label=tlabel, color=fillcolors[idx%len(fillcolors)], edgecolor='black', hatch=patterns[idx % len(patterns)]);
        idx += 1;
    ax.grid();
    ax.set_xticks([(width * i + len(data.keys()) * bar_width / 2) for i in range(len(labels))])
    ax.set_xticklabels(labels)
    if bbox_to_anchor:
        ax.legend(bbox_to_anchor=(1.01, 1.0),fontsize=12)
    else:
        ax.legend(loc=0,fontsize=12)
    plt.savefig(outfile);
    plt.savefig(outfile+'.pdf');
    plt.close('all');

def plotBar(data, key, value, width=1, xlabel="", ylabel="", title="", outfile="out.png",
xlogscale=False, ylogscale=False, xmax=None, ymax=None, xunit=1.0, yunit=1.0):
    fig = plt.figure(figsize=(7,4))
    ax = plt.subplot(111)
    plt.subplots_adjust(left=0.1, bottom=0.15, right=0.7, top=0.9, wspace=0, hspace=0)
    ax.set_title(title);
    ax.set_xlabel(xlabel);
    ax.set_ylabel(ylabel);
    if not xmax is None:
        ax.set_xlim(0,xmax);
    if not ymax is None:
        ax.set_ylim(0,ymax);
    if xlogscale:
        ax.set_xscale('symlog')
    if ylogscale:
        ax.set_yscale('symlog')
    idx = 0;
    bar_width = float(width) / (len(data.keys()) + 1)
    for test in sortKeys(data.keys()):
        if len(data[test]) == 0:
            continue;
        x = [(v[key] + idx*bar_width) / xunit for v in data[test]];
        y = [v[value] / yunit for v in data[test]];
        ax.bar(x,y, width=bar_width, label=test.replace('_', ' '), color=linecolors[idx%len(linecolors)], edgecolor=linecolors[idx%len(linecolors)]);
        idx += 1;
    ax.grid();
    ax.legend(bbox_to_anchor=(1.01, 1.0))
    plt.savefig(outfile);
    plt.savefig(outfile+'.pdf');
    plt.close('all');

def plotCDF(data, key=None, value=None, xlabel="", ylabel="CDF", title="", outfile="out.png", xrange=None,
        xlogscale=False, raw=False, nmarkers=11, small=False):
    fig = None
    ax = None
    if small:
        fig = plt.figure(figsize=(4,4))
        ax = plt.subplot(111)
        plt.subplots_adjust(left=0.2, bottom=0.2, right=0.95, top=0.98, wspace=0, hspace=0)
    else:
        fig = plt.figure(figsize=(8,5))
        ax = plt.subplot(111)
        plt.subplots_adjust(left=0.15, bottom=0.15, right=0.95, top=0.95, wspace=0, hspace=0)
    ax.set_title(title, fontsize=20);
    ax.set_xlabel(xlabel, fontsize=16);
    ax.set_ylabel(ylabel, fontsize=16);
    ax.tick_params(labelsize=12);
    if not xrange is None:
        ax.set_xlim(xrange[0],xrange[1]);
    if not raw:
        ax.set_ylim(0,1.05);
    if xlogscale:
        ax.set_xscale('symlog')
    idx = 0;
    for test in sortKeys(data.keys()):
        if value is not None and key is not None:
            x = [v[value] for v in data[test][key]];
        elif value is not None and key is None:
            x = [v[value] for v in data[test]];
        elif value is None and key is not None:
            x = [v for v in data[test][key]];
        else:
            x = [v for v in data[test]]
        x.sort();
        if len(x) == 0:
            continue;
        if not raw:
            y = [float(i) / len(x) for i in range(len(x))];
        else:
            y = [i for i in range(len(x))];
        label = test.replace("KCOV", "").replace('_', ' ').strip()
        marker = markers[int(idx%len(markers))] if nmarkers > 1 else None
        markevery = int((len(x)-1) / (nmarkers-1)) if nmarkers > 1 else None
        if markevery == 0:
            markevery = 1
        ax.plot(x,y, label=label, color=linecolors[idx%len(linecolors)], linestyle=linestyles[int(idx/len(linecolors))], marker=marker, markersize=8, markevery=markevery);
        idx += 1;
    ax.legend(loc=0, fontsize=12)
    ax.grid();
    plt.savefig(outfile);
    plt.savefig(outfile + '.pdf');
    plt.close('all');

if __name__ == "__main__":
    # data_FP = [0.01086956522, 0.01176470588, 0.01204819277, 0.01282051282, 0.01282051282, 0.02857142857, 0.04166666667, 0.0447761194, 0.05882352941, 0.06451612903, 0.1612903226, 0.9166666667, 0.9285714286, 1, 1] 
    data_FP = [0.8846153846, 0.8947368421, 0.9873417722, 0.8421052632, 0.8, 0.7368421053, 0.8333333333, 0.9756097561, 0.775, 0.8, 0.8260869565, 0.5428571429, 0.6666666667, 0.8461538462, 0.8947368421, 0.8666666667, 0.3333333333, 0.962962963, 0.9759036145, 0.8275862069, 0.64, 0.9743589744, 0.9166666667, 0.006369426752, 0.7619047619, 0.975, 0.5833333333, 0.9294117647, 0.75, 0.8095238095, 0.6111111111, 0.9444444444, 0.987654321, 0.9411764706, 0.8888888889, 0.6428571429, 0.7692307692, 0.6086956522, 0.987654321, 0.009174311927, 0.9759036145, 0.9411764706, 0.9473684211, 0.9615384615, 0.04761904762, 0.06818181818, 0.08181818182, 0.6842105263, 0.1294117647, 0.7857142857, 0.09649122807, 0.015625, 0.1941747573, 0.9620253165, 0.9638554217, 0.7368421053, 0.09734513274, 0.04166666667, 0.05194805195, 0.75, 0.0826446281, 0.08108108108, 0.04545454545, 0.9855072464, 0.0979020979, 0.8947368421, 0.03191489362, 0.07792207792, 0.06060606061]
    # data_TP = [0.009174311927, 0.625, 0.00395256917, 0.04545454545, 0.9666666667, 0.2424242424, 0.03076923077, 0.1764705882, 0.03333333333, 0.02564102564, 0.0843373494, 0.1842105263, 0.9836065574, 1]
    data_TP = [0.935483871, 0.9634146341, 0.07407407407, 0.02702702703, 0.08695652174, 0.04347826087, 0.08333333333, 0.07692307692, 0.9090909091, 0.08695652174, 0.04347826087, 0.935483871, 0.76, 0.01834862385, 0.9666666667, 0.9565217391, 0.9302325581, 0.7948717949, 0.9189189189, 0.935483871, 0.90625, 0.9677419355, 0.8611111111, 0.8947368421, 0.9756097561, 0.9743589744, 1, 1, 0.9638554217, 0.9310344828, 0.908045977, 0.9651162791, 0.9756097561, 0.9642857143, 0.9736842105, 0.8974358974, 0.8656716418, 0.9253731343, 0.953125, 0.9682539683, 0.9090909091, 0.9230769231, 0.7407407407, 0.9230769231, 0.8939393939, 0.9253731343, 0.8309859155, 0.5412844037, 0.875, 0.9104477612, 0.8714285714, 0.9538461538, 0.96875, 0.5042016807, 0.9076923077, 0.9523809524, 1, 0.3833992095, 0.390438247, 0.3855421687, 0.384]
    data = {"TP": [(v,0) for v in data_TP], "FP": [(v,0) for v in data_FP]}
    plotCDF(data, 0, xlabel="len(newSig) / len(inputSig)", ylabel="CDF", title="Stability Threshold", outfile="cdf.png")

