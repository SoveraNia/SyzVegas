// CREATED: Daimeng Wang

package main

import (
	_ "encoding/hex"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/syzkaller/pkg/hash"
	"github.com/google/syzkaller/pkg/log"
	"github.com/google/syzkaller/pkg/rpctype"
	_ "github.com/google/syzkaller/sys"
)

type MABGMTStatus struct {
	MABMu             sync.RWMutex
	MABGamma          float64       // No reset
	MABEta            float64       // No reset
	MABRound          int           // How many MAB choices have been made. No reset
	MABExp31Round     int           // How many rounds of Exp3.1. No reset
	MABExp31Threshold float64       // Threshold based on Round. No sync
	MABGLC            []rpctype.GLC // {Generate, Mutate, Triage}. Used for stationary bandit
	MABMaxLoss        float64
	MABMinLoss        float64
	MABMaxGain        float64
	MABMinGain        float64
	MABMaxCost        float64
	MABMinCost        float64
	MABWindowGain     []float64
	MABWindowLoss     []float64
	MABWindowCost     []float64
	MABTriageInfo     map[hash.Sig]*rpctype.TriageInfo
}

func MinMax(arr []float64) (float64, float64) {
	min := 0.0
	max := 0.0
	for i, v := range arr {
		if i == 0 || v < min {
			min = v
		}
		if i == 0 || v > max {
			max = v
		}
	}
	return min, max
}

func (fuzzer *Fuzzer) MABEstimateGain(gain float64, pr float64) float64 {
	if fuzzer.fuzzerConfig.MABAlgorithm == "Exp3-IX" {
		ret := gain / (pr + fuzzer.MABGamma)
		if fuzzer.fuzzerConfig.MABVerbose {
			fuzzer.writeLog("- MAB Estimate Gain: %v / (%v + %v) = %v\n", gain, pr, fuzzer.MABGamma, ret)
		}
		return ret
	} else {
		ret := gain / pr
		if fuzzer.fuzzerConfig.MABVerbose {
			fuzzer.writeLog("- MAB Estimate Gain: %v / %v = %v\n", gain, pr, ret)
		}
		return ret
	}
}

func (fuzzer *Fuzzer) MABGetWeight(lock bool) []float64 {
	// Stationary bandit
	// algorithm := fuzzer.fuzzerConfig.MABAlgorithm
	if lock {
		fuzzer.MABMu.Lock()
		defer fuzzer.MABMu.Unlock()
	}
	x := []float64{0.0, 0.0, 0.0}
	weight := []float64{1.0, 1.0, 1.0}
	eta := fuzzer.MABEta
	const (
		MABWeightThresholdMax = 1.0e+300
		MABWeightThresholdMin = 1.0e-300
	)
	x[0] = eta * fuzzer.MABGLC.NormalizedGenerate.TotalGain
	x[1] = eta * fuzzer.MABGLC.NormalizedMutate.TotalGain
	x[2] = eta * fuzzer.MABGLC.NormalizedTriage.TotalGain
	fuzzer.writeLog("- MABWeight %v\n", x)
	// Compute median to prevent overflow
	median := x[0]
	if x[0] > x[1] {
		if x[1] > x[2] {
			median = x[1]
		} else if x[0] > x[2] {
			median = x[2]
		}
	} else {
		if x[1] < x[2] {
			median = x[1]
		} else if x[0] < x[2] {
			median = x[2]
		}
	}
	for i := 0; i <= 2; i++ {
		weight[i] = math.Exp(x[i] - median)
		if weight[i] > MABWeightThresholdMax {
			weight[i] = MABWeightThresholdMax
		}
		if weight[i] < MABWeightThresholdMin {
			weight[i] = MABWeightThresholdMin
		}
	}
	// fuzzer.writeLog("- MABWeight [%v, %v, %v], Round %v\n", weight[0], weight[1], weight[2], fuzzer.MABRound)
	return weight
}

func (fuzzer *Fuzzer) MABReset() {
	// fuzzer.MABMu.Lock()
	// defer fuzzer.MABMu.Unlock()
	fuzzer.MABMaxGain = 0.0
	fuzzer.MABMinGain = math.Inf(0)
	fuzzer.MABMaxLoss = 0.0
	fuzzer.MABMinLoss = math.Inf(0)
	fuzzer.MABMaxCost = 0.0
	fuzzer.MABMinCost = math.Inf(0)
	// Don't reset MABGLC[4] for Nael's algorithm
	fuzzer.MABGLC.NormalizedGenerate = rpctype.GLC{}
	fuzzer.MABGLC.NormalizedMutate = rpctype.GLC{}
	fuzzer.MABGLC.NormalizedTriage = rpctype.GLC{}
	fuzzer.MABGLC.NaelAll = rpctype.GLC{}
	// These are for normalization only. Should reset upon Exp3.1 round?
	fuzzer.MABWindowGain = fuzzer.MABWindowGain[:0]
	fuzzer.MABWindowLoss = fuzzer.MABWindowLoss[:0]
	fuzzer.MABWindowCost = fuzzer.MABWindowCost[:0]
}

func (fuzzer *Fuzzer) __MABNormalizeNael(gain float64, cost float64, glcRaw *rpctype.GLC) float64 {
	g := 0.0
	if glcRaw.TotalGain > 0 {
		g = gain*(glcRaw.TotalCost/glcRaw.TotalGain) - cost
		if fuzzer.fuzzerConfig.MABVerbose {
			fuzzer.writeLog("- MAB Nael Gain: %v * %v / %v - %v = %v\n", gain, glcRaw.TotalCost, glcRaw.TotalGain, cost, g)
		}
	}
	// }
	return g
}

func (fuzzer *Fuzzer) MABNormalizeNael(gain float64, cost float64) float64 {
	return fuzzer.__MABNormalizeNael(gain, cost, &fuzzer.MABGLC.RawAll)
}

func (fuzzer *Fuzzer) MABNormalizeCorpusNael(gain float64, cost float64) float64 {
	return fuzzer.__MABNormalizeNael(gain, cost, &fuzzer.MABGLC.RawMutate)
}

func ZLogistic(x float64, n int, sum float64, sum2 float64, offset float64) (float64, float64, float64) {
	// mean = sum / n, std = sqrt((sum2 / n) - mean^2)
	// z = (x - mean - offset) / std
	// x = 1 / (1 + e^-z)
	if n == 0 {
		return 0.5, 0.0, 0.0
	}
	mean := sum / float64(n)
	tmp := (sum2 / float64(n)) - (mean * mean)
	if tmp < 0.0 {
		fmt.Printf("- MAB WTF: Cannot compute sqrt(%v)\n", tmp)
		return 0.5, 0.0, 0.0
	}
	std := math.Sqrt(tmp)
	if std == 0.0 {
		return 0.5, 0.0, 0.0
	}
	z := (x - mean - offset) / std
	ret := 1.0 / (1.0 + math.Exp(-z))
	return ret, mean, std
}

func (fuzzer *Fuzzer) __MABNormalizeGLC(gain float64, glcNael *rpctype.GLC) float64 {
	x := 0.0
	x_mean, x_std := 0.0, 0.0
	offset := 0.0
	// Update min max and GLC
	if fuzzer.fuzzerConfig.MABNormalize >= 0 {
		// Use Z-score + logistic function, but handle 0 differently
		if glcNael.Count > 0 {
			offset = -glcNael.TotalGain / float64(glcNael.Count)
			// Allow negative gain for Nael's Algorithm
			x, x_mean, x_std = ZLogistic(gain, glcNael.Count, glcNael.TotalGain, glcNael.TotalGain2, offset)
			x = (2.0 * x) - 1.0
			if fuzzer.fuzzerConfig.MABVerbose {
				fuzzer.writeLog("- MAB Normalized Gain: %v, (%v,%v)\n", x, x_mean, x_std)
			}
		}
	} else {
		return gain
	}
	return x
}

func (fuzzer *Fuzzer) MABNormalizeGLC(gain float64) float64 {
	return fuzzer.__MABNormalizeGLC(gain, &fuzzer.MABGLC.NaelAll)
}

func (fuzzer *Fuzzer) MABNormalizeCorpusGLC(gain float64) float64 {
	return fuzzer.__MABNormalizeGLC(gain, &fuzzer.MABGLC.NaelMutate)
}

func (fuzzer *Fuzzer) MABIncrementCorpusMutateCount(idx int, count int) {
	if idx < 0 || idx > len(fuzzer.corpus) {
		fuzzer.writeLog("- MAB Error: idx = %v\n", idx)
		return
	}
	fuzzer.MABMu.Lock()
	defer fuzzer.MABMu.Unlock()
	fuzzer.corpus[idx].MABMutateCount += count
	if fuzzer.fuzzerConfig.MABVerbose {
		sig := hash.Hash(fuzzer.corpus[idx].Serialize())
		fuzzer.writeLog("- Mutate Count %v: %v, +%v, %v\n", idx, sig.String(), count, fuzzer.corpus[idx].MABMutateCount)
	}
}

func (fuzzer *Fuzzer) MABUpdateCorpusWeight(pidx int, x float64) {
	// fuzzerSnapshot := fuzzer.snapshot()
	// K := len(fuzzer.corpus)
	pr := fuzzer.corpusPrios
	// Normalize
	// Update gain/loss
	// if fuzzer.fuzzerConfig.MABSeedSelection == "Exp3-Gain" {
	// 	fuzzer.corpus[pidx].MABMutateGainNormOrig += x / pr[pidx] / float64(K)
	// } else {
	fuzzer.corpus[pidx].MABMutateGainNormOrig += x / (pr[pidx] + fuzzer.MABCorpusGamma)
	// }
	// Update corpus selection weight
	MABWeightThresholdMax := float64(math.Exp(64))
	MABWeightThresholdMin := float64(math.Exp(-64))
	// algorithm := fuzzer.fuzzerConfig.MABSeedSelection
	eta := fuzzer.MABCorpusEta
	/*
		if algorithm == "Exp3-Gain" {
			eta = fuzzer.MABCorpusGamma / float64(K)
		}
	*/
	prio := 1.0
	prio = math.Exp(eta * fuzzer.corpus[pidx].MABMutateGainNormOrig)
	if prio > MABWeightThresholdMax {
		prio = MABWeightThresholdMax
	}
	if prio < MABWeightThresholdMin {
		prio = MABWeightThresholdMin
	}
	if fuzzer.fuzzerConfig.MABVerbose {
		fuzzer.writeLog("- MAB Corpus %v, %v: %v -> %v\n", pidx, fuzzer.corpus[pidx].MABMutateGainNormOrig, fuzzer.corpusPrios[pidx], prio)
	}
	fuzzer.corpusPrios[pidx] = prio
	// Normalize prios. Handle explicit exploration here
	/* For Exp3-Gain. It's complicates since the probability is not proportinal to weight/prio
	gamma := fuzzer.MABCorpusGamma
	if s > 0.0 {
		for i, _ := range fuzzer.corpus {
			if algorithm == "Exp3-Gain" {
				fuzzer.corpusPrios[i] = (1-gamma)*fuzzer.corpusPrios[i]/s + gamma/float64(K)
			}
			fuzzer.sumPrios = fuzzer.sumPrios + fuzzer.corpusPrios[i]
			fuzzer.corpusPriosSum[i] = fuzzer.sumPrios
		}
	}
	*/
	if pidx == 0 {
		fuzzer.corpusPriosSum[pidx] = fuzzer.corpusPrios[pidx]
	} else {
		fuzzer.corpusPriosSum[pidx] = fuzzer.corpusPriosSum[pidx-1] + fuzzer.corpusPrios[pidx]
	}
	for i := pidx + 1; i < len(fuzzer.corpus); i++ {
		fuzzer.corpusPriosSum[i] = fuzzer.corpusPriosSum[i-1] + fuzzer.corpusPrios[i]
	}
	fuzzer.sumPrios = fuzzer.corpusPriosSum[len(fuzzer.corpus)-1] // We need this to prevent any leakage due to float computation
}

func (fuzzer *Fuzzer) MABUpdateWeightUnstableAssocNael(itemType int, result interface{}, pr []float64, K int) {
	if itemType == 2 && fuzzer.fuzzerConfig.MABAlgorithm != "N/A" {
		_r, ok := result.(TriageResult)
		if !ok {
			return
		}
		gain := float64(_r.minimizeGainRaw)
		cost_ver := float64(_r.verifyTime)
		cost_min := float64(_r.minimizeTime)
		time_save := float64(_r.minimizeTimeSave)
		pidx := _r.pidx
		cost_before_min := _r.sourceCost
		// Update triage cost for corpus
		_gain := fuzzer.MABNormalizeNael(gain, cost_ver+cost_min)
		x := fuzzer.MABNormalizeGLC(_gain)
		_x := fuzzer.MABEstimateGain(x, pr[2])
		fuzzer.MABGLC.NormalizedTriage.Update(_x, 0.0)
		fuzzer.MABGLC.NaelAll.Update(_gain, 0.0)
		// fuzzer.MABUpdateWindow(_gain, 0.0, 0.0)
		// Record triage cost
		if _r.success && pidx >= 0 && pidx < len(fuzzer.corpus) {
			fuzzer.corpus[pidx].MABTriageGainNorm = _gain
			fuzzer.corpus[pidx].MABVerifyGain = _r.verifyGainRaw
			fuzzer.corpus[pidx].MABMinimizeGain = _r.minimizeGainRaw
			fuzzer.corpus[pidx].MABVerifyCost = cost_ver
			fuzzer.corpus[pidx].MABMinimizeCost = cost_min
			fuzzer.corpus[pidx].MABMinimizeTimeSave = time_save
			fuzzer.corpus[pidx].MABCostBeforeMinimize = cost_before_min
			// Mark for update
			fuzzer.MABCorpusUpdate[pidx] = 1
		}
		fuzzer.MABGLC.RawAll.Update(gain, cost_ver+cost_min)
	} else if itemType == 0 && fuzzer.fuzzerConfig.MABAlgorithm != "N/A" {
		_r, ok := result.(ExecResult)
		if !ok {
			return
		}
		gain := float64(_r.gainRaw)
		cost := float64(_r.time)
		// Normalize
		_gain := fuzzer.MABNormalizeNael(gain, cost)
		x := fuzzer.MABNormalizeGLC(_gain)
		_x := fuzzer.MABEstimateGain(x, pr[0])
		fuzzer.MABGLC.NormalizedGenerate.Update(_x, 0.0)
		// fuzzer.MABUpdateWindow(_gain, 0.0, 0.0)
		fuzzer.MABGLC.NaelAll.Update(_gain, 0.0)
		fuzzer.MABGLC.RawAll.Update(gain, cost)
	} else if itemType == 1 {
		_r, ok := result.(ExecResult)
		if !ok {
			return
		}
		gain := float64(_r.gainRaw)
		cost := float64(_r.time)
		pidx := _r.pidx
		if pidx < 0 || pidx > len(fuzzer.corpus) {
			fuzzer.writeLog("- MAB Error: pidx = %v\n", pidx)
			return
		}
		if fuzzer.fuzzerConfig.MABAlgorithm != "N/A" {
			mutate_cnt := fuzzer.corpus[pidx].MABMutateCount
			cost_ver := fuzzer.corpus[pidx].MABVerifyCost
			cost_min := fuzzer.corpus[pidx].MABMinimizeCost
			gain_min := fuzzer.corpus[pidx].MABMinimizeGain
			gain_ver := fuzzer.corpus[pidx].MABVerifyGain
			gain_mut_cur := fuzzer.corpus[pidx].MABMutateGain + gain // Current total raw gain
			cost_mut_cur := fuzzer.corpus[pidx].MABMutateCost + cost // Current total raw cost of mutation
			n_mut_prev := fuzzer.corpus[pidx].MABMutateGainNorm      // Prev Nael-normalized gain for mutation
			n_tri_prev := fuzzer.corpus[pidx].MABTriageGainNorm      // Prev Nael-normalized cost for mutation
			// cost_mut_time_save := float64(mutate_cnt)*fuzzer.corpus[pidx].MABCostBeforeMinimize - cost_mut_cur
			cost_mut_time_save := float64(mutate_cnt) * fuzzer.corpus[pidx].MABMinimizeTimeSave
			if cost_mut_cur+cost_ver == 0.0 {
				fuzzer.writeLog("- MAB Error: cost_ver = %v, cost_mut = %v\n", cost_ver, cost_mut_cur)
				fuzzer.MABGLC.RawAll.Update(gain, cost)
				fuzzer.MABGLC.RawMutate.Update(gain, cost)
				return
			}
			// Distribut gain considering minimize effect
			// Minimize
			n_min_cur := fuzzer.MABNormalizeNael(gain_min, 0.0)
			if fuzzer.fuzzerConfig.MABVerbose {
				// fuzzer.writeLog("- MAB Assoc Minimize Gain %v: %v * %v - (%v + %v) = %v\n", mutate_cnt, mutate_cnt, fuzzer.corpus[pidx].MABCostBeforeMinimize, fuzzer.corpus[pidx].MABMutateCost, cost, cost_mut_time_save)
				fuzzer.writeLog("- MAB Assoc Minimize Gain %v: %v + %v * %v = %v\n", mutate_cnt, n_min_cur, mutate_cnt, fuzzer.corpus[pidx].MABMinimizeTimeSave, n_min_cur+cost_mut_time_save)
			}
			n_min_cur = n_min_cur + cost_mut_time_save
			// Stablize
			n_ver_cur := gain_mut_cur*cost_ver/(cost_mut_cur+cost_ver) + gain_ver
			if fuzzer.fuzzerConfig.MABVerbose {
				fuzzer.writeLog("- MAB Assoc Verify Gain %v: (%v + %v) * %v / %v + %v = %v\n", mutate_cnt, fuzzer.corpus[pidx].MABMutateGain, gain, cost_ver, cost_mut_cur+cost_ver, gain_ver, n_ver_cur)
			}
			n_ver_cur = fuzzer.MABNormalizeNael(n_ver_cur, 0.0)
			// Triage
			n_tri_cur := n_ver_cur + n_min_cur - (cost_ver + cost_min)
			if fuzzer.fuzzerConfig.MABVerbose {
				fuzzer.writeLog("- MAB Assoc Triage Gain %v: (%v + %v) - (%v + %v) = %v\n", mutate_cnt, n_ver_cur, n_min_cur, cost_ver, cost_min, n_tri_cur)
			}
			// Mutation
			n_mut_cur := gain_mut_cur * cost_mut_cur / (cost_mut_cur + cost_ver)
			if fuzzer.fuzzerConfig.MABVerbose {
				fuzzer.writeLog("- MAB Assoc Mutate Gain %v: (%v + %v) * (%v + %v) / %v = %v\n", mutate_cnt, fuzzer.corpus[pidx].MABMutateGain, gain, fuzzer.corpus[pidx].MABMutateCost, cost, cost_mut_cur+cost_ver, n_mut_cur)
			}
			n_mut_cur = fuzzer.MABNormalizeNael(n_mut_cur, cost_mut_cur)
			// Compute x
			n_mut_diff := n_mut_cur - n_mut_prev
			n_tri_diff := n_tri_cur - n_tri_prev
			if fuzzer.fuzzerConfig.MABVerbose {
				fuzzer.writeLog("- MAB Assoc Triage Gain Diff: %v - %v = %v\n", n_tri_cur, n_tri_prev, n_tri_diff)
				fuzzer.writeLog("- MAB Assoc Mutate Gain Diff: %v - %v = %v\n", n_mut_cur, n_mut_prev, n_mut_diff)
			}
			x_mut_diff := fuzzer.MABNormalizeGLC(n_mut_diff)
			x_tri_diff := fuzzer.MABNormalizeGLC(n_tri_diff)
			_x_mut := fuzzer.MABEstimateGain(x_mut_diff, pr[1])
			// Triage might be unavailable this time, as a result, compute triage's probality as if triage is available
			// Fortunately, we know that mutation is definitely available
			_x_tri := fuzzer.MABEstimateGain(x_tri_diff, pr[1])
			fuzzer.MABGLC.NormalizedMutate.Update(_x_mut, 0.0)
			fuzzer.MABGLC.NormalizedTriage.Update(_x_tri, 0.0)
			// Update program stat
			fuzzer.corpus[pidx].MABMutateGain = gain_mut_cur
			fuzzer.corpus[pidx].MABMutateCost = cost_mut_cur
			fuzzer.corpus[pidx].MABMutateGainNorm = n_mut_cur
			fuzzer.corpus[pidx].MABTriageGainNorm = n_tri_cur
			// Don't use associated gain for normalization
			n_norm := fuzzer.MABNormalizeNael(gain, cost)
			fuzzer.MABGLC.NaelAll.Update(n_norm, 0.0)
		}
		// Mark for update
		fuzzer.MABCorpusUpdate[pidx] = 1
		// Update for seed selection MAB
		if fuzzer.fuzzerConfig.MABSeedSelection == "Exp3-Gain" || fuzzer.fuzzerConfig.MABSeedSelection == "Exp3-IX" {
			n_norm1 := fuzzer.MABNormalizeCorpusNael(gain, cost)
			x_norm := fuzzer.MABNormalizeCorpusGLC(n_norm1)
			fuzzer.MABUpdateCorpusWeight(pidx, x_norm)
			fuzzer.MABGLC.NaelMutate.Update(n_norm1, 0.0)
		}
		fuzzer.corpus[pidx].MABMutateCount += 1 // With MAB, always one mutation
		if fuzzer.fuzzerConfig.MABVerbose {
			__sig := hash.Hash(fuzzer.corpus[pidx].Serialize())
			fuzzer.writeLog("- Mutate Count %v: %v, +1, %v\n", pidx, __sig.String(), fuzzer.corpus[pidx].MABMutateCount)
		}
		fuzzer.MABGLC.RawAll.Update(gain, cost)
		fuzzer.MABGLC.RawMutate.Update(gain, cost)
	} else {
		fuzzer.writeLog("- MAB Error: %v\n", itemType)
	}
}

func (fuzzer *Fuzzer) MABPreprocessResult(result interface{}) interface{} {
	// Deal with cost outlier
	cost_max := 5000000000.0 / fuzzer.fuzzerConfig.MABTimeUnit

	switch result.(type) {
	case ExecResult:
		{
			_r, ok := result.(ExecResult)
			if ok {
				if _r.time > cost_max {
					_r.time = cost_max
				} else if _r.time < 0.0 {
					_r.time = 0.0
				}
			}
			return _r
		}
	case TriageResult:
		{
			_r, ok := result.(TriageResult)
			if ok {
				if _r.verifyTime > cost_max {
					_r.verifyTime = cost_max
				} else if _r.verifyTime < 0.0 {
					_r.verifyTime = 0.0
				}
				if _r.minimizeTime > cost_max {
					_r.minimizeTime = cost_max
				} else if _r.minimizeTime < 0.0 {
					_r.minimizeTime = 0.0
				}
				if _r.minimizeTimeSave > cost_max || _r.minimizeTimeSave < -cost_max {
					_r.minimizeTimeSave = 0
				}
			}
			return _r
		}
	default:
		log.Fatalf("unknown result type: %#v", result)
	}
	return result
}

func (fuzzer *Fuzzer) MABBootstrapExp31() {
	fuzzer.MABGamma = math.Exp2(float64(-fuzzer.MABExp31Round))
	fuzzer.MABEta = fuzzer.MABGamma / 3.0
	if fuzzer.fuzzerConfig.MABAlgorithm == "Exp3-IX" {
		fuzzer.MABEta = 2.0 * fuzzer.MABGamma
	}
	fuzzer.MABExp31Threshold = 3.0 * math.Log(3.0) * math.Exp2(2.0*float64(fuzzer.MABExp31Round)) / (math.E - 1.0)
	fuzzer.MABExp31Threshold = fuzzer.MABExp31Threshold - (3.0 / fuzzer.MABGamma)
	fuzzer.writeLog("- MAB Exp3.1 New Round %v, Gamma: %v, Eta: %v, Threshold: %v\n", fuzzer.MABExp31Round, fuzzer.MABGamma, fuzzer.MABEta, fuzzer.MABExp31Threshold)
}

func (fuzzer *Fuzzer) MABUpdateWeight(itemType int, result interface{}, pr []float64, K int) {
	ts0 := time.Now().UnixNano()
	// 0 = Generate, 1 = Mutate, 2 = Triage
	if K == 0 {
		fuzzer.writeLog("- MAB Error: K = %v\n", 0)
		return
	}
	if itemType < 0 || itemType > 2 || len(pr) < 3 {
		fuzzer.writeLog("- MAB Error: itemType = %v\n", itemType)
		return
	}
	if pr[itemType] == 0 {
		fuzzer.writeLog("- MAB Error: pr[%v] = 0\n", itemType)
		return
	}
	fuzzer.MABMu.Lock()

	defer func() {
		// Update MAB status no matter what path we choose
		if fuzzer.fuzzerConfig.MABVerbose {
			fuzzer.writeLog("- MAB Round %v GLC: %+v\n", fuzzer.MABRound, fuzzer.MABGLC)
		}
		// fuzzer.MABRound += 1
		if fuzzer.fuzzerConfig.MABExp31 {
			Exp31_max := 0.0
			Exp31_min := math.Inf(0)
			// Consider gain only. Since Nael's gain can be negative, use abs()
			Exp31_max = math.Max(fuzzer.MABGLC.NormalizedGenerate.TotalGain, math.Max(fuzzer.MABGLC.NormalizedMutate.TotalGain, fuzzer.MABGLC.NormalizedTriage.TotalGain))
			Exp31_min = math.Min(fuzzer.MABGLC.NormalizedGenerate.TotalGain, math.Min(fuzzer.MABGLC.NormalizedMutate.TotalGain, fuzzer.MABGLC.NormalizedTriage.TotalGain))
			if Exp31_max-Exp31_min > fuzzer.MABExp31Threshold || Exp31_max > fuzzer.MABExp31Threshold || math.Abs(Exp31_min) > fuzzer.MABExp31Threshold {
				fuzzer.MABExp31Round += 1
				fuzzer.MABReset()
				fuzzer.MABBootstrapExp31()
			}
		}
		ts1 := time.Now().UnixNano()
		fuzzer.writeLog("- MAB Update: %v\n", ts1-ts0)
		fuzzer.MABMu.Unlock()
	}()
	_result := fuzzer.MABPreprocessResult(result)
	if !fuzzer.fuzzerConfig.MABTargetCorpus {
		fuzzer.MABUpdateWeightUnstableAssocNael(itemType, _result, pr, K)
		return
	}
}

func (fuzzer *Fuzzer) getMABStatus() (rpctype.RPCMABStatus, int64) {
	ts0 := time.Now().UnixNano()
	fuzzer_status := rpctype.RPCMABStatus{
		Round:      fuzzer.MABRound,
		Exp31Round: fuzzer.MABExp31Round,
		MABGLC:     fuzzer.MABGLC,
		CorpusGLC:  make(map[hash.Sig]rpctype.CorpusGLC),
		TriageInfo: fuzzer.MABTriageInfo,
	}
	for pidx, _ := range fuzzer.MABCorpusUpdate {
		if pidx >= 0 && pidx < len(fuzzer.corpus) {
			p := fuzzer.corpus[pidx]
			sig := hash.Hash(p.Serialize())
			fuzzer_status.CorpusGLC[sig] = rpctype.CorpusGLC{
				Smashed:            p.Smashed,
				MutateCount:        p.MABMutateCount,
				VerifyGain:         p.MABVerifyGain,
				VerifyCost:         p.MABVerifyCost,
				MinimizeGain:       p.MABMinimizeGain,
				MinimizeCost:       p.MABMinimizeCost,
				MinimizeTimeSave:   p.MABMinimizeTimeSave,
				MutateGain:         p.MABMutateGain,
				MutateCost:         p.MABMutateCost,
				CostBeforeMinimize: p.MABCostBeforeMinimize,
				MutateGainNorm:     p.MABMutateGainNorm,
				MutateGainNormOrig: p.MABMutateGainNormOrig,
				TriageGainNorm:     p.MABTriageGainNorm,
			}
			// fuzzer.writeLog("- MAB Corpus Sync %v, %v > %v,%v,%v,%v,%v\n", sig.String(), p.MABMutateCount, p.MABMutateGain, p.MABMutateCost, p.MABVerifyCost, p.MABMinimizeCost, p.MABMinimizeTimeSave)
		}
	}
	fuzzer.MABCorpusUpdate = make(map[int]int) // Clear map
	t := time.Now().UnixNano() - ts0
	return fuzzer_status, t
}

func (fuzzer *Fuzzer) writeMABStatus(manager_status rpctype.RPCMABStatus) int64 {
	ts0 := time.Now().UnixNano()
	if fuzzer.MABRound < manager_status.Round {
		fuzzer.MABRound = manager_status.Round
		fuzzer.MABExp31Round = manager_status.Exp31Round
		if fuzzer.fuzzerConfig.MABExp31 {
			// Need to update gamma/eta for Exp31
			fuzzer.MABBootstrapExp31()
		}
		fuzzer.MABGLC = manager_status.MABGLC
		for sig, v := range manager_status.TriageInfo {
			if v.TriageCount <= 0 {
				continue
			}
			if _, ok := fuzzer.MABTriageInfo[sig]; !ok {
				fuzzer.MABTriageInfo[sig] = &rpctype.TriageInfo{}
			}
			fuzzer.MABTriageInfo[sig].Source = v.Source
			fuzzer.MABTriageInfo[sig].SourceCost = v.SourceCost
			fuzzer.MABTriageInfo[sig].TriageGain = v.TriageGain
			fuzzer.MABTriageInfo[sig].VerifyCost = v.VerifyCost
			fuzzer.MABTriageInfo[sig].VerifyGain = v.VerifyGain
			fuzzer.MABTriageInfo[sig].MinimizeTimeSave = v.MinimizeTimeSave
			fuzzer.MABTriageInfo[sig].MinimizeGain = v.MinimizeGain
			fuzzer.MABTriageInfo[sig].MinimizeCost = v.MinimizeCost
			fuzzer.MABTriageInfo[sig].TriageCount = v.TriageCount
			fuzzer.MABTriageInfo[sig].TriageTotal = v.TriageTotal
			fuzzer.MABTriageInfo[sig].SourceGainNorm = v.SourceGainNorm
			fuzzer.MABTriageInfo[sig].TriageGainNorm = v.TriageGainNorm
			fuzzer.writeLog("- MAB TriageInfo Sync %v: %+v\n", sig.String(), fuzzer.MABTriageInfo[sig])
		}
	}
	// fuzzer.writeLog("- MAB Corpus Sync: %v/%v,%v\n", len(manager_status.CorpusGLC), len(fuzzer.corpusHashes), len(fuzzer.corpus))
	for sig, v := range manager_status.CorpusGLC {
		pidx := -1
		ok := false
		if pidx, ok = fuzzer.corpusHashes[sig]; ok && pidx >= 0 && pidx < len(fuzzer.corpus) {
			fuzzer.corpus[pidx].Smashed = v.Smashed
			fuzzer.corpus[pidx].MABVerifyGain = v.VerifyGain
			fuzzer.corpus[pidx].MABVerifyCost = v.VerifyCost
			fuzzer.corpus[pidx].MABMinimizeGain = v.MinimizeGain
			fuzzer.corpus[pidx].MABMinimizeCost = v.MinimizeCost
			fuzzer.corpus[pidx].MABMinimizeTimeSave = v.MinimizeTimeSave
			fuzzer.corpus[pidx].MABMutateCost = v.MutateCost
			fuzzer.corpus[pidx].MABMutateGain = v.MutateGain
			fuzzer.corpus[pidx].MABMutateGainNorm = v.MutateGainNorm
			fuzzer.corpus[pidx].MABMutateGainNormOrig = v.MutateGainNormOrig
			fuzzer.corpus[pidx].MABTriageGainNorm = v.TriageGainNorm
			fuzzer.corpus[pidx].MABCostBeforeMinimize = v.CostBeforeMinimize
			// fuzzer.writeLog("- MAB Corpus Sync %v, %v, %v < %v,%v,%v,%v,%v\n", sig.String(), fuzzer.corpus[pidx].MABMutateCount, v.MutateCount, v.MutateGain, v.MutateCost, v.VerifyCost, v.MinimizeCost, v.MinimizeTimeSave)
			fuzzer.corpus[pidx].MABMutateCount = v.MutateCount
		}
	}
	t := time.Now().UnixNano() - ts0
	return t
}
