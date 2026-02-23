#!/usr/bin/env python3
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


import sys
from functools import reduce
from math import sqrt
from argparse import ArgumentParser, ArgumentTypeError
from multiprocessing import cpu_count
from pathlib import Path

def lcm(a, b):
    # This doesn't work on python2...  Should get updated at some point
    from math import gcd
    l = abs(a*b) // gcd(a, b)
    return l

def lcm_array(arr):
    return reduce((lambda x,y: lcm(x, y)), arr)


def write_HPL_input(N, NB, PQ_Grids, outputfile='HPL.dat'):
# Problem sizes (Line 6)
# https://icl.utk.edu/hpl/faq/index.html#122
# Memory per Rank... ~sqrt(MpR/8 *.75)  'div by 8 to get # doubles'
#N = [1000 10000 10000]


# Block sizes (Line 8)
# https://icl.utk.edu/hpl/faq/index.html#124
#NB = [32 44 48 64 .. 256]


# Ps and Qs
# https://icl.utk.edu/hpl/faq/index.html#125
# 1:k, with k in [1..3] range
# depends on # ranks... P*Q = #numRanks used
#PQ_Grids = [(2, 2), (P, Q)...]

    hpl_input = {
        'numN': len(N),
        'Ns': " ".join([str(n) for n in N]),
        'numNB': len(NB),
        'NBs':  " ".join([str(nb) for nb in NB]),
        'numPQ': len(PQ_Grids),
        'Ps': " ".join([str(int(x[0])) for x in PQ_Grids]),
        'Qs': " ".join([str(int(x[1])) for x in PQ_Grids])
    }


    hpl_template="""HPLinpack benchmark input file
Innovative Computing Laboratory, University of Tennessee
HPL.out      output file name (if any)
8            device out (6=stdout,7=stderr,file)
{numN}       # of problems sizes (N)
{Ns}         Ns
{numNB}            # of NBs
{NBs}           NBs
0            PMAP process mapping (0=Row-,1=Column-major)
{numPQ}            # of process grids (P x Q)
{Ps}            Ps
{Qs}            Qs
16.0         threshold
1            # of panel fact
2            PFACTs (0=left, 1=Crout, 2=Right)
1            # of recursive stopping criterium
4            NBMINs (>= 1)
1            # of panels in recursion
2            NDIVs
1            # of recursive panel fact.
1            RFACTs (0=left, 1=Crout, 2=Right)
1            # of broadcast
1            BCASTs (0=1rg,1=1rM,2=2rg,3=2rM,4=Lng,5=LnM)
1            # of lookahead depth
1            DEPTHs (>=0)
2            SWAP (0=bin-exch,1=long,2=mix)
64           swapping threshold
0            L1 in (0=transposed,1=no-transposed) form
0            U  in (0=transposed,1=no-transposed) form
1            Equilibration (0=no,1=yes)
8            memory alignment in double (> 0)
"""

    with open(outputfile, 'w') as fp:
        fp.write(hpl_template.format(**hpl_input))


def mem_per_core():
    from multiprocessing import cpu_count
    nCPU = cpu_count()

    MemTotal = None
    with open("/proc/meminfo", 'r') as fp:
        for line in fp:
            info = line.split()
            if 'MemTotal:' == info[0]:
                MemTotal = int(info[1]) / 1024
                break
    if not MemTotal:
        raise Exception('Unable to determine system memory.  Please specify --mem_per_rank')

    return int(MemTotal / nCPU)


def calculate_N(nRanks, MemPerRank, memWeight):
    totalMemInBytes = MemPerRank * nRanks * (memWeight / 100.) * 1024*1024
    totalDoubles = totalMemInBytes / 8
    nSize = int(sqrt(totalDoubles))
    # TODO:  Maybe round something "human"
    return nSize


def parse_ratio(string):
    try:
        (p,q) = [int(x) for x in string.split(':')]
        return [p, q]
    except:
        msg = "%r should be of format <int>:<int> (ie, '1:2')"%string
        raise ArgumentTypeError(msg)

def estimate_PQ(nRanks):
    factors = [(x, int(nRanks/x)) for x in range(2, int(sqrt(nRanks))+1) if nRanks%x == 0]
    if len(factors) == 0:
        factors = [(1, nRanks)]

    return factors[-1]



def create_input(ranks, mem_weight):

    N = calculate_N(ranks, mem_per_core(), mem_weight)
    NB = [ 128, 160, 192, 256] # Some "reasonable" numbers
    LCM = lcm_array(NB)
    factor = int(N / LCM)
    if factor%2:
        factor -=1
    N = [int(LCM * factor)]

    PQ = [estimate_PQ(ranks)]
    
    write_HPL_input(N, NB, PQ)



def parse_hpl_out(outFile):
    results=[]
    with outFile.open('r') as fp:
        line = fp.readline()
        while line:
            line = line.strip()
            if ['T/V', 'N', 'NB', 'P', 'Q', 'Time', 'Gflops'] == line.split():
                # Skip the next line (all '-----'), and get our data
                fp.readline()
                line = fp.readline().strip()
                vals = line.split()
                results.append(float(vals[-1]))
            line = fp.readline()
    if len(results) > 0:
        return {'result_unit': 'Gflops', 'result_value': max(results)}
    return None
    


if __name__ == '__main__':
    import os, subprocess, json

    nranks=int(os.environ.get("SLURM_NTASKS", 1))

    create_input(nranks, mem_weight=25.0)
    # Now, run HPL itself
    subprocess.run(["mpirun", "xhpl"])

    outFile = Path('./HPL.out')
    if outFile.exists():
        # Create KPI.json
        kpi = parse_hpl_out(outFile)
        if kpi:
            with open('kpi.json', 'w') as fp:
                fp.write(json.dumps(kpi))
