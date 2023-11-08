#!/bin/bash 
ticker=("GOOG" "AMZN" "MSFT" "NVDA" "META" "TSLA" "PEP" "COST")
echo "BI:" $BATCH_TASK_INDEX
echo "TI: ${ticker[$BATCH_TASK_INDEX]}"
python3 -m pip install -r /mnt/disks/fsi/mc_run_reqs.txt
python3 /mnt/disks/fsi/mc_run.py \
  --ticker ${ticker[$BATCH_TASK_INDEX]} \
  --iterations 500 \
  --start_date 2022-01-01
