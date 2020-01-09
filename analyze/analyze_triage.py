import sys
import os
import copy
import traceback
import simplejson as json
import matplotlib.pyplot as plt
from matplotlib.collections import PatchCollection
from matplotlib.patches import Rectangle
import numpy as np
from typing import List

from sklearn.feature_extraction.text import TfidfVectorizer, CountVectorizer
from sklearn.neighbors import KNeighborsClassifier
from sklearn.model_selection import KFold, ShuffleSplit
from sklearn.dummy import DummyClassifier
from sklearn.naive_bayes import GaussianNB
from sklearn.naive_bayes import MultinomialNB
from sklearn.neural_network import MLPClassifier
from sklearn.svm import SVC

from syscalls import syscalls
from utils import loadDataCached
from plot import plot, plotCDF
from prog import Prog

def __parseCall(call):
    name = call.split('(')[0]
    if '=' in name:
        name = name.split('=')[1].strip()
    args = call.split(name)[1]
    return name.lower(), args

def __processTest(test):
    """
    Things to collect:
        Times triaging failed programs
        signalRun: Number. Remaining sig for TP, FP.
        Minimization: Number. Time spent on failed programs. Success TP, FP. Fail TN, FN.
        Minimization success stats
    """
    ret = []
    fn = 'result_' + test
    if not os.path.isfile(fn):
        return ret;
    f = open(fn);
    prev_pc = 0;
    executeCount = 0; 
    idx = 0;
    ts_cur = 0;
    ts_bgn = 0;
    status_cur = {
        "triagingTotal": 0,
        "triagingFail": 0,
        "minimizeTotal": 0,
        "minimizeFail": 0,
        "minimizeNew": 0,
        "minimizeTP": 0,
        "minimizeFP": 0,
        "minimizeTN": 0,
        "minimizeFN": 0, 
    }
    minimizeSz = [{}, {}]
    sigInit = 0;
    corpusProg = False
    # Minimize
    progStatus = None
    inProg = False
    curCalls = []
    curProg = None
    minimizeProgFrom = None
    minimizeProgTo = None
    minimizeAttempts = []
    minimizeSuccess = False
    minimizeExec = 0
    # Coverage
    coverageTotal = set()
    coveragePrev = 0
    for line in f:
        line = line.strip('\n').strip();
        if len(line) == 0:
            continue;
        if line[:3] == '<<<' and line[-3:] == '>>>':
            line = line.strip('<<<').strip('>>>')
            ts_cur = int(line)
            if ts_bgn == 0:
                ts_bgn = ts_cur
        elif (line == ">" or line[:3] == ">>>") and not inProg:
            inProg = True
            curCalls = []
        elif line == "<" or line == "<<<":
            inProg = False
            curProg = Prog.newProg(calls=curCalls, ts=ts_cur, signal=0, origin=None)
            if progStatus == "MinimizeFrom":
                minimizeProgFrom = curProg
            elif progStatus == "MinimizeAttempt":
                minimizeProgTo = curProg
            progStatus = None
        elif inProg:
            if line[:2] == "> ":
                line = line.strip("> ")
            if len(line) == 0:
                continue
            curCalls.append(' '.join(__parseCall(line)))
        elif line[:2] == '= ':
            tmp = line.split();
            try:
                pc = int(tmp[1], 16)
            except:
                continue
            if (pc & 0xffff000000000000) == 0xffff000000000000:
                coverageTotal.add(pc);
            elif (pc & 0xffffffff00000000) == 0:
                coverageTotal.add(pc);
        elif line[:2] == "- " and "executeRaw" in line:
            executeCount += 1;
            status = copy.deepcopy(status_cur);
            status["executeCount"] = executeCount;
            status["ts"] = (ts_cur - ts_bgn) / 1000000000;
            ret.append(status)
            coveragePrev = len(coverageTotal)
        elif "# signalRun 0: " in line:
            tmp = line.split("# signalRun 0: ")[1].split('+');
            sigInit = int(tmp[0])
        elif line[:8] == "# Result":
            tmp = line.strip("# Result: ").split(',')
            if int(tmp[2]) > 0:
                corpusProg = True
            else:
                corpusProg = False
                status_cur["triagingFail"] += 1;
            # print(tmp[2], status_cur["triagingFail"])
            status_cur["triagingTotal"] += 1;
        elif line[-8:] == "Minimize":
            progStatus = "MinimizeFrom"
        elif "# Minimize Attempt" in line:
            progStatus = "MinimizeAttempt"
        elif "# Minimize Fail" in line or "# Minimize Success" in line:
            minimizeProgFrom.childrenMinimize.append(minimizeProgTo)
            entry = {
                    "from": minimizeProgFrom,
                    "to": minimizeProgTo,
                    "success": "Success" in line
            }
            minimizeAttempts.append(entry)	
        elif "# Minimize" in line and "->" in line:
            tmp = line.split(': ')[1].replace('->',' ').replace('+', ' ').replace(',', ' ').split();
            if tmp[3] == tmp[5]:
                minimizeSuccess = True
            minimizeExec += 1
            if len(coverageTotal) > coveragePrev:
                 status_cur["minimizeNew"] += 1
    f.close();
    return ret, minimizeAttempts;

def CrossValidation(data, y, vocabulary=syscalls, train_size=0.2, batch=10000, mode=None, test_name=""):
    idx_bgn = 0
    idx_end = batch
    models = ["Dummy", "NB", "NB-I", "SVM", "KNN", "NN"]
    scores = {}
    number_total = {}
    for m in models:
        scores[m] = []
        number_total[m] = [0,0,0,0]
    clf_NBI = MultinomialNB()
    vectorizer_NBI = CountVectorizer(vocabulary=vocabulary)
    while idx_bgn < len(data):
        __data = data[idx_bgn:idx_end]
        __y = y[idx_bgn:idx_end]
        scores_local = {}
        number_local = {}
        for m in models:
            scores_local[m] = []
            number_local[m] = [0,0,0,0]
        #kf = KFold(n_splits=K)
        #for train_index, test_index in kf.split(__data):
            # Reverse
            # d_train = [__data[i] for i in test_index]
            # d_test = [__data[i] for i in train_index]
            # y_test, y_train = y[train_index], y[test_index]
        try:
            if type(train_size) == float:
                split_point = int(batch * train_size)
                if split_point > len(__data):
                    continue
            elif type(train_size) == int and train_size < len(__data):
                split_point = train_size
            else:
                continue
            d_train = __data[:split_point]
            d_test = __data[split_point:]
            y_train = __y[:split_point]
            y_test = __y[split_point:]
            if mode == "TF-IDF":
                vectorizer = TfidfVectorizer()
                X_train = vectorizer.fit_transform(d_train).toarray()
                X_test = vectorizer.transform(d_test).toarray()
            elif mode == "Count":
                vectorizer = CountVectorizer()
                X_train = vectorizer.fit_transform(d_train).toarray()
                X_test = vectorizer.transform(d_test).toarray()
                #X_train_NBI = vectorizer_NBI.transform(d_train).toarray()
                #X_test_NBI = vectorizer_NBI.transform(d_test).toarray()
            else:
                X_train = np.array(d_train)
                X_test = np.array(d_test)
                X_train_NBI = X_train
                X_test_NBI = X_test
            # Dummy
            sys.stderr.write("Dummy ")
            clf_dummy = DummyClassifier(strategy='uniform')
            clf_dummy.fit(X_train, y_train)
            pred = clf_dummy.predict(X_test)
            for i in range(len(pred)):
                if pred[i] == 1 and y_test[i] == 1:
                    number_local["Dummy"][0] += 1
                elif pred[i] == 0 and y_test[i] == 0:
                    number_local["Dummy"][1] += 1
                elif pred[i] == 1 and y_test[i] == 0:
                    number_local["Dummy"][2] += 1
                elif pred[i] == 0 and y_test[i] == 1:
                    number_local["Dummy"][3] += 1
            scores_local["Dummy"].append(clf_dummy.score(X_test, y_test))
            # NB
            sys.stderr.write("NB ")
            clf_NB = MultinomialNB()
            clf_NB.fit(X_train, y_train)
            pred = clf_NB.predict(X_test)
            for i in range(len(pred)):
                if pred[i] == 1 and y_test[i] == 1:
                    number_local["NB"][0] += 1
                elif pred[i] == 0 and y_test[i] == 0:
                    number_local["NB"][1] += 1
                elif pred[i] == 1 and y_test[i] == 0:
                    number_local["NB"][2] += 1
                elif pred[i] == 0 and y_test[i] == 1:
                    number_local["NB"][3] += 1
            scores_local["NB"].append(clf_NB.score(X_test, y_test))
            # NB-I
            sys.stderr.write("NB-I ")
            if (mode != "TF-IDF" and mode != "Count"):
              clf_NBI.partial_fit(X_train_NBI, y_train, classes=[0,1])
              X_increment = []
              y_increment = []
              for i in range(len(X_test)):
                pred = clf_NBI.predict([X_test_NBI[i]])[0]
                if pred == 1 and y_test[i] == 1:
                    number_local["NB-I"][0] += 1
                elif pred == 0 and y_test[i] == 0:
                    number_local["NB-I"][1] += 1
                elif pred == 1 and y_test[i] == 0:
                    number_local["NB-I"][2] += 1
                elif pred == 0 and y_test[i] == 1:
                    number_local["NB-I"][3] += 1
              scores_local["NB-I"].append((number_local["NB-I"][0] + number_local["NB-I"][1]) / len(X_test))
            # SVM
            # SVM is too slow for TF-IDF or Term Count
            sys.stderr.write("SVM ")
            if (mode != "TF-IDF" and mode != "Count"):
              clf_SVN = SVC(gamma='auto')
              clf_SVN.fit(X_train, y_train)
              pred = clf_SVN.predict(X_test)
              for i in range(len(pred)):
                if pred[i] == 1 and y_test[i] == 1:
                    number_local["SVM"][0] += 1
                elif pred[i] == 0 and y_test[i] == 0:
                    number_local["SVM"][1] += 1
                elif pred[i] == 1 and y_test[i] == 0:
                    number_local["SVM"][2] += 1
                elif pred[i] == 0 and y_test[i] == 1:
                    number_local["SVM"][3] += 1
              scores_local["SVM"].append(clf_SVN.score(X_test, y_test))
            # KNN
            # KNN is too slow for TF-IDF or Term Count
            sys.stderr.write("KNN ")
            if (mode != "TF-IDF" and mode != "Count"):
              clf_KNN = KNeighborsClassifier(n_neighbors=1)
              clf_KNN.fit(X_train, y_train)
              pred = clf_KNN.predict(X_test)
              for i in range(len(pred)):
                if pred[i] == 1 and y_test[i] == 1:
                    number_local["KNN"][0] += 1
                elif pred[i] == 0 and y_test[i] == 0:
                    number_local["KNN"][1] += 1
                elif pred[i] == 1 and y_test[i] == 0:
                    number_local["KNN"][2] += 1
                elif pred[i] == 0 and y_test[i] == 1:
                    number_local["KNN"][3] += 1
              scores_local["KNN"].append(clf_KNN.score(X_test, y_test))
            # NN 
            # NN is too slow for TF-IDF or Term Count
            sys.stderr.write("KNN ")
            if (mode != "TF-IDF" and mode != "Count"):
              clf_NN = MLPClassifier(solver='lbfgs', alpha=1e-5, hidden_layer_sizes=(5, 2), random_state=1)
              clf_NN.fit(X_train, y_train)
              pred = clf_NN.predict(X_test)
              for i in range(len(pred)):
                if pred[i] == 1 and y_test[i] == 1:
                    number_local["NN"][0] += 1
                elif pred[i] == 0 and y_test[i] == 0:
                    number_local["NN"][1] += 1
                elif pred[i] == 1 and y_test[i] == 0:
                    number_local["NN"][2] += 1
                elif pred[i] == 0 and y_test[i] == 1:
                    number_local["NN"][3] += 1
              scores_local["NN"].append(clf_NN.score(X_test, y_test))
            sys.stderr.write("DONE\n")
        except:
            traceback.print_exc()
            idx_bgn += batch
            idx_end += batch
            continue
        idx_bgn += batch
        idx_end += batch
        for model in scores_local:
            scores[model] += scores_local[model]
            if len(scores_local[model]) > 0:
                scores_local[model] = np.mean(scores_local[model])
        for model in number_total:
            for i in range(4):
                number_total[model][i] += number_local[model][i]
        # print("%f\t%f\t%f\t%f\t%f" % (len(X_test), number_local["NB-TN"], number_local["NB-FN"], number_local["SVM-TN"], number_local["SVM-FN"]))
    for model in scores:
        if len(scores[model]) > 0:
            scores[model] = np.mean(scores[model])
        else:
            scores[model] = -1
    for model in number_total:
        for i in range(4):
            number_total[model][i] /= len(data)
    out = "%s\t%d" % (test_name, len(data))
    for m in models:
        out += "\t%f\t%f\t%f\t%f\t%f" % (
                number_total[m][0], number_total[m][1], number_total[m][2], number_total[m][3], scores[m]
            )
    print(out)
    sys.stdout.flush()

def MLMinimize(attempts):
    if len(attempts) < 100:
        return
    data_success = [(1 if d["success"] is True else 0) for d in attempts]
    y = np.array(data_success)
    data_2 = [
        [
            len(d["from"]), len(d["to"])
        ] for d in attempts]
    data_4 = [
        [
            len(d["from"]), len(d["to"]),
            d["from"].argCount, d["to"].argCount,
        ] for d in attempts]
    data_6 = [
        [
            len(d["from"]), len(d["to"]),
            d["from"].argCount, d["to"].argCount,
            d["from"].argSize, d["to"].argSize
        ] for d in attempts]
    # CrossValidation(data, y)
    # TF-IDF
    '''
    data_from = []
    data_to = []
    data_all = []
    data_diff = []
    for d in attempts:
        tmp_from = []
        for call in d["from"].calls:
            tmp_from.append(call.split()[0])
        data_from.append(" ".join(tmp_from))
        tmp_to = []
        for call in d["to"].calls:
            tmp_to.append(call.split()[0])
        data_to.append(" ".join(tmp_to))
        tmp_all = []
        tmp_diff = []
        for call in tmp_from:
            tmp_all.append("FROM_" + call)
            if not call in tmp_to:
                tmp_diff.append(call)
        for call in tmp_to:
            tmp_all.append("TO_" + call)
        data_all.append(" ".join(tmp_all))
        data_diff.append(" ".join(tmp_diff))
    voc_all = []
    for call in syscalls:
        voc_all.append("FROM_" + call)
        voc_all.append("TO_" + call)
    print(tmp_from)
    print(tmp_to)
    print(tmp_all)
    print(tmp_diff)
    '''
    batch = 1000000000
    for train_size in [1000, 2500, 10000, 20000]:
        print("Batch: Inf, Train: %d" % train_size)
        CrossValidation(data_2, y, batch=batch, train_size=train_size, test_name="2-feats")
        CrossValidation(data_4, y, batch=batch, train_size=train_size, test_name="4-feats")
        CrossValidation(data_6, y, batch=batch, train_size=train_size, test_name="6-feats")
        #CrossValidation(data_from, y, batch=batch, train_size=train_size, mode="Count", test_name="TC-From")
        #CrossValidation(data_to, y, batch=batch, train_size=train_size, mode="Count", test_name="TC-To")
        #CrossValidation(data_all, y, batch=batch, train_size=train_size, mode="Count", test_name="TC-All")
        #CrossValidation(data_diff, y, batch=batch, train_size=train_size, mode="Count", test_name="TC-Diff")
    for batch in [10000, 20000]:
      for train_size in [0.05, 0.1, 0.2]:
        print("Batch: %d, Train: %f" % (batch, train_size))
        CrossValidation(data_2, y, batch=batch, train_size=train_size, test_name="2-feats")
        CrossValidation(data_4, y, batch=batch, train_size=train_size, test_name="4-feats")
        CrossValidation(data_6, y, batch=batch, train_size=train_size, test_name="6-feats")
        #CrossValidation(data_from, y, batch=batch, train_size=train_size, mode="Count", test_name="TC-From")
        #CrossValidation(data_to, y, batch=batch, train_size=train_size, mode="Count", test_name="TC-To")
        #CrossValidation(data_all, y, batch=batch, train_size=train_size, mode="Count", test_name="TC-All", vocabulary=voc_all)
        #CrossValidation(data_diff, y, batch=batch, train_size=train_size, mode="Count", test_name="TC-Diff")

def analyzeMinimize(test_name, attempts):
    data_from = {}
    data_to = {}
    for d in attempts:
        from_id = d["from"]
        to_id = d["to"]
        if not from_id in data_from:
            data_from[from_id] = []
        if not to_id in data_to:
            data_to[to_id] = []
        data_from[from_id].append(d["success"])
        data_to[to_id].append(d["success"])
    # print(data_from)
    success_rate_from = []
    for from_id in data_from:
        count = 0.0
        score = 0.0
        for success in data_from[from_id]:
            if success:
                score += 1.0
            count += 1.0
        success_rate_from.append(score / count)
    print(success_rate_from)
    plotCDF({"Success Rate": success_rate_from}, xlabel="Success Rate", ylabel="CDF", outfile="minimize_success_%s.png" % test_name)

def plotTriage(tests=["RAMINDEX", "KCOV"]):
    data = {}
    for test in tests:
        #__data, minimizeAttempts = loadDataCached('triage_%s.cache', test, __processTest);
        __data, minimizeAttempts = __processTest(test);
        print(len(__data), __data[-1] if len(__data) > 0 else -1)
        # Triaging
        data = {
            "Total": [(v["executeCount"], v["ts"], v["triagingTotal"]) for v in __data],
            "Wasted": [(v["executeCount"], v["ts"], v["triagingFail"]) for v in __data],
        }
        plot(data, 0, 2, xlabel="Programs executed", ylabel="Number", title="", outfile="triage_total_%s.png" % test);
        # Minimization total
        if "Default" in test:
            print(test)
            # MLMinimize(minimizeAttempts)
            # analyzeMinimize(test, minimizeAttempts)
            exit()
        #plot(data, 0, 2, xlabel="Programs executed", ylabel="Number", title="", outfile="minimize_accuracy_%s.png" % test);

