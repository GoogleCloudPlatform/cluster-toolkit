#!/bin/perl
# Copyright 2021 Google LLC
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

my $min = 80;
my $failed = 0;
while (<>){
  print $_;
  if ( $_ =~ /coverage: (\d+\.\d)%/ ) {
    $failed++ if ($1 < $min);
  }
}
if ($failed > 0) {
   print STDERR "coverage must be above $min%, $failed packages were below that.\n";
   exit 1
}
