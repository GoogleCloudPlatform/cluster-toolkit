// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package validators

import(
	"context"
	"fmt"

	"google.golang.org/api/compute/v1"
)

func GetAvailableReservationCount(ctx context.Context,
	project, zone, reservationName string) (int, error) {

	svc, err := compute.NewService(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to create compute service: %w", err)
	}

	reservation, err := svc.Reservations.Get(project, zone, reservationName).Do()
	if err != nil {
		return 0,fmt.Errorf("failed tp get reservation: %w", err)
	}

	count := reservation.SpecificReservation.Count 
	used := reservation.SpecificReservation.InUseCount

	var available int = int(count - used)

	return available, nil
}