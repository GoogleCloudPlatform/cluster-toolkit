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

# TODO: raise ./cmd min coverage to 80% after tests are written
my $min = 80;
my $cmdmin = 40;
my $shellmin = 0;
my $validatorsmin = 25;
my $failed_coverage = 0;

while (<>){
  print $_;
  if ( $_ =~ /hpc-toolkit\/cmd.*coverage: (\d+\.\d)%/) {
    $failed_coverage++ if ($1 < $cmdmin);
  } elsif ( $_ =~ /hpc-toolkit\/pkg\/shell.*coverage: (\d+\.\d)%/) {
    $failed_coverage++ if ($1 < $shellmin);
  } elsif ( $_ =~ /hpc-toolkit\/pkg\/validators.*coverage: (\d+\.\d)%/) {
    $failed_coverage++ if ($1 < $validatorsmin);  
  } elsif ( $_ =~ /coverage: (\d+\.\d)%/ ) {
    $failed_coverage++ if ($1 < $min);
  }
}

if ($failed_coverage > 0) {
   print STDERR "Coverage must be above $cmdmin% for ./cmd and $min% for other packages, $failed_coverage packages were below that.\n";
   exit 1
}
