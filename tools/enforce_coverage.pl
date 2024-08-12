#!/usr/bin/perl
# Copyright 2022 Google LLC
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

use strict;
use warnings;

my @failed;
while (<>){
  print $_;

  my @thresholds = qw(
    cmd 40
    pkg/shell 0
    pkg/logging 0
    pkg/validators 10
    pkg/inspect 60
    pkg/modulewriter 79
    pkg 80
  );

  while (@thresholds) {
    my ($path, $threshold) = splice(@thresholds, 0, 2);
    if ( $_ =~ /hpc-toolkit\/$path.*coverage: (\d+\.\d)%/) {
      chomp, push @failed, "$_ <= $threshold%\n" if ($1 < $threshold);
      last;
    }
  }
}

if (@failed) {
   print STDERR "\nFAILED:\n@failed";
   exit 1
}
