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