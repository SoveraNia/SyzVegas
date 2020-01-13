package glc

import ()

type GLC struct {
	Count     int
	TotalGain float64
	// TotalLoss  float64
	TotalCost  float64
	TotalGain2 float64
	// TotalLoss2 float64
	TotalCost2 float64
}

type MABGLC struct {
	// For Scheduling
	NormalizedGenerate GLC // Normalized gain for Generate. Used for weight deciding
	NormalizedMutate   GLC // Normalized gain for Mutate. Used for weight deciding
	NormalizedTriage   GLC // Normalized gain for Triage. Used for weight deciding
	RawAll             GLC // Raw gain/cost for all Gen/Mut/Tri. Used for computing Nael's Normallization
	NaelAll            GLC // Nael-Normalized gain/cost for all Gen/Mut/Tri. Used for normalization
	// For Seed selection
	RawMutate  GLC // Raw gain/cost for mutations. Used for Nael's computation for seed selection
	NaelMutate GLC // Nael-Normalized gain/cost for mutations. Used for normalization
}

type TriageInfo struct {
	Source           int
	SourceCost       float64
	TriageGain       float64
	VerifyGain       float64
	VerifyCost       float64
	MinimizeGain     float64
	MinimizeCost     float64
	MinimizeTimeSave float64
	TriageCount      int
	TriageTotal      int
	SourceGainNorm   float64
	TriageGainNorm   float64
}

type CorpusGLC struct {
	Smashed            bool // Whether this seed has been smashed
	MutateCount        int
	Cost               float64
	MutateCost         float64 // Total cost of mutating this seed
	MutateGain         float64 // Total gain of mutating this seed
	VerifyGain         float64
	VerifyCost         float64
	MinimizeGain       float64
	MinimizeCost       float64
	MinimizeTimeSave   float64 // Time save due to minimization
	CostBeforeMinimize float64 // Cost before minimization
	MutateGainNorm     float64 // Normalized gain after Nael's
	MutateGainNormOrig float64 // Normalized gain after Nael's
	TriageGainNorm     float64 // Normalized gain after Nael's
}

func (glc *GLC) Update(gain float64, cost float64) {
	const Max = 1.0e+100

	glc.Count += 1
	glc.TotalGain += gain
	glc.TotalGain2 += gain * gain
	glc.TotalCost += cost
	glc.TotalCost2 += cost * cost
	if glc.TotalGain > Max {
		glc.TotalGain = Max
	}
	if glc.TotalGain2 > Max {
		glc.TotalGain2 = Max
	}
	if glc.TotalCost > Max {
		glc.TotalCost = Max
	}
	if glc.TotalCost2 > Max {
		glc.TotalCost2 = Max
	}
}

func (glc *GLC) Remove(gain float64, cost float64) {
	glc.Count -= 1
	glc.TotalGain -= gain
	glc.TotalGain2 -= gain * gain
	glc.TotalCost -= cost
	glc.TotalCost2 -= cost * cost
}
