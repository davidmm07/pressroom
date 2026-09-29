package tools

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/davidmm07/pressroom/internal/domain"
)

// Job is one order's run through the factory.
type Job struct {
	ID       string    `json:"jobId"`
	OrderID  string    `json:"orderId"`
	Product  string    `json:"product"`
	Quantity int       `json:"quantity"`
	Station  string    `json:"currentStation"`
	Status   string    `json:"jobStatus"` // QUEUED, PRINTING or DONE
	QueuedAt time.Time `json:"queuedAt"`
	ShipBy   time.Time `json:"shipBy"`
}

// Station is a press or finishing line.
type Station struct {
	ID          string   `json:"station"`
	Products    []string `json:"products"`
	UnitsPerDay int      `json:"unitsPerDay"`
}

// Factory is the manufacturing execution system: production jobs and the
// stations that print them.
type Factory interface {
	Job(ctx context.Context, id string) (Job, error)
	Stations(ctx context.Context) ([]Station, error)
	OpenJobs(ctx context.Context) ([]Job, error) // queued or printing, oldest first
}

// JobPlan says whether a job will ship on time and where it could move.
type JobPlan struct {
	JobID           string   `json:"jobId"`
	OrderID         string   `json:"orderId"`
	Product         string   `json:"product"`
	Quantity        int      `json:"quantity"`
	CurrentStation  string   `json:"currentStation"`
	ShipBy          string   `json:"shipBy"`
	ProjectedDoneOn string   `json:"projectedDoneOn"`
	Late            bool     `json:"late"`
	TargetStation   string   `json:"targetStation,omitempty"`
	TargetDoneOn    string   `json:"targetDoneOn,omitempty"`
	Recommendation  string   `json:"recommendation"`
	NotNeeded       []string `json:"notNeeded"`
}

// PlanJob projects when a job finishes from the queue ahead of it and looks
// for another station that would finish it by its ship-by date. Exported
// for tests and reuse.
func PlanJob(job Job, stations []Station, open []Job, today time.Time) JobPlan {
	p := JobPlan{
		JobID: job.ID, OrderID: job.OrderID, Product: job.Product, Quantity: job.Quantity,
		CurrentStation: job.Station, ShipBy: job.ShipBy.Format(time.DateOnly), NotNeeded: []string{},
	}
	if job.Status == "DONE" {
		p.ProjectedDoneOn, p.Recommendation = "done", "Already printed; nothing to do."
		p.NotNeeded = []string{"reroute_job", "notify_team"}
		return p
	}
	today = today.UTC().Truncate(24 * time.Hour)
	finish := func(units, perDay int) time.Time {
		return today.AddDate(0, 0, (units+perDay-1)/perDay)
	}
	backlog, ahead := map[string]int{}, 0 // units waiting per station; units ahead of this job
	for _, o := range open {
		if o.ID == job.ID {
			continue
		}
		backlog[o.Station] += o.Quantity
		if o.Station == job.Station && o.QueuedAt.Before(job.QueuedAt) {
			ahead += o.Quantity
		}
	}

	done := time.Time{}
	for _, s := range stations {
		if s.ID == job.Station && s.UnitsPerDay > 0 {
			done = finish(ahead+job.Quantity, s.UnitsPerDay)
		}
	}
	if done.IsZero() {
		p.ProjectedDoneOn, p.Late = "unknown", true
	} else {
		p.ProjectedDoneOn, p.Late = done.Format(time.DateOnly), done.After(job.ShipBy)
	}
	if !p.Late {
		p.Recommendation = "On track to finish before the ship-by date."
		p.NotNeeded = []string{"reroute_job", "notify_team"}
		return p
	}

	best := time.Time{}
	for _, s := range stations { // at the back of another station's queue
		if s.ID == job.Station || s.UnitsPerDay <= 0 || !slices.Contains(s.Products, job.Product) {
			continue
		}
		if at := finish(backlog[s.ID]+job.Quantity, s.UnitsPerDay); !at.After(job.ShipBy) && (best.IsZero() || at.Before(best)) {
			best, p.TargetStation = at, s.ID
		}
	}
	if p.TargetStation == "" {
		p.Recommendation = "No station can finish it by the ship-by date. Tell the customer and upgrade the shipping."
		p.NotNeeded = []string{"reroute_job"}
		return p
	}
	p.TargetDoneOn = best.Format(time.DateOnly)
	p.Recommendation = fmt.Sprintf("Move it to %s to finish on %s, in time to ship.", p.TargetStation, p.TargetDoneOn)
	return p
}

// ProductionStatus checks a job the factory flagged as at risk.
func ProductionStatus(f Factory) Tool {
	return Func(domain.ToolSpec{
		Name: "production_status",
		Description: "Check a production job: when it will finish given the queue ahead of it, whether it misses its ship-by date, " +
			"and which station could finish it in time instead.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {"jobId": {"type": "string", "pattern": "^J-[0-9]{1,8}$"}},
		  "required": ["jobId"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in struct {
		JobID string `json:"jobId"`
	}) (any, error) {
		job, err := f.Job(ctx, in.JobID)
		if err != nil {
			return nil, err
		}
		stations, err := f.Stations(ctx)
		if err != nil {
			return nil, err
		}
		open, err := f.OpenJobs(ctx)
		if err != nil {
			return nil, err
		}
		return PlanJob(job, stations, open, time.Now()), nil
	})
}

type rerouteInput struct {
	JobID         string `json:"jobId"`
	TargetStation string `json:"targetStation"`
	Reason        string `json:"reason"`
}

// RerouteJob moves a job to another station. It reshuffles the factory
// floor, so the production lead approves it.
func RerouteJob(c Commerce, f Factory) Tool {
	return Func(domain.ToolSpec{
		Name:             "reroute_job",
		RequiresApproval: true,
		Description:      "Move a production job to another station that prints the same product. Requires approval from the production lead.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {
		    "jobId":         {"type": "string", "pattern": "^J-[0-9]{1,8}$"},
		    "targetStation": {"type": "string", "pattern": "^[A-Z]+-[0-9]{1,3}$"},
		    "reason":        {"type": "string", "minLength": 5, "maxLength": 500}
		  },
		  "required": ["jobId","targetStation","reason"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in rerouteInput) (any, error) {
		job, err := f.Job(ctx, in.JobID)
		if err != nil {
			return nil, err
		}
		stations, err := f.Stations(ctx)
		if err != nil {
			return nil, err
		}
		i := slices.IndexFunc(stations, func(s Station) bool { return s.ID == in.TargetStation })
		switch {
		case i < 0:
			return nil, fmt.Errorf("station %s: %w", in.TargetStation, ErrNotFound)
		case in.TargetStation == job.Station:
			return nil, fmt.Errorf("job %s is already on %s", job.ID, job.Station)
		case !slices.Contains(stations[i].Products, job.Product):
			return nil, fmt.Errorf("%s does not print %s (it prints %v)", in.TargetStation, job.Product, stations[i].Products)
		}
		id, err := c.Record(ctx, "REROUTE", idempotencyKey(ctx, "reroute"), in)
		if err != nil {
			return nil, err
		}
		return map[string]any{"rerouteId": id, "jobId": job.ID, "from": job.Station, "to": in.TargetStation, "status": "MOVED"}, nil
	})
}

// unitCostCents is the material and labour cost of one 3-inch unit. These
// numbers are illustrative; production reads them from the pricing service.
var unitCostCents = map[string]int{"STICKERS": 26, "LABELS": 18, "MAGNETS": 48, "BUTTONS": 35}

// minDealMargin keeps every deal at a 20% gross margin or better.
const minDealMargin = 0.20

// DealFloorCents is the lowest price for a pack that keeps the margin.
func DealFloorCents(product string, packSize int) int {
	return int(math.Ceil(float64(unitCostCents[product]*packSize) / (1 - minDealMargin)))
}

// LineForecast is the spare capacity of one product line.
type LineForecast struct {
	Product       string  `json:"product"`
	CapacityUnits int     `json:"capacityUnits"`
	BookedUnits   int     `json:"bookedUnits"`
	IdleUnits     int     `json:"idleUnits"`
	Utilization   float64 `json:"utilization"`
}

// DealSuggestion is a starting point the model may adjust.
type DealSuggestion struct {
	Product    string `json:"product"`
	PackSize   int    `json:"packSize"`
	PriceCents int    `json:"priceCents"`
	FloorCents int    `json:"floorCents"`
	Days       int    `json:"days"`
}

type Forecast struct {
	HorizonDays   int             `json:"horizonDays"`
	Lines         []LineForecast  `json:"lines"`
	SuggestedDeal *DealSuggestion `json:"suggestedDeal,omitempty"`
	NotNeeded     []string        `json:"notNeeded"`
}

// dealPack is the pack size deals are built around.
const dealPack = 50

// ForecastCapacity compares each product line's capacity over the horizon
// with the work already booked, and suggests a deal for the idlest line
// when idle presses are worth filling. Exported for tests and reuse.
func ForecastCapacity(stations []Station, open []Job, days int) Forecast {
	capacity, booked := map[string]int{}, map[string]int{}
	for _, s := range stations { // a station splits its time across its products
		for _, p := range s.Products {
			capacity[p] += s.UnitsPerDay * days / len(s.Products)
		}
	}
	for _, j := range open {
		booked[j.Product] += j.Quantity
	}
	f := Forecast{HorizonDays: days, Lines: []LineForecast{}, NotNeeded: []string{}}
	var best LineForecast
	for _, p := range []string{"STICKERS", "LABELS", "MAGNETS", "BUTTONS", "PACKAGING", "TSHIRTS"} {
		if capacity[p] == 0 {
			continue
		}
		l := LineForecast{Product: p, CapacityUnits: capacity[p], BookedUnits: booked[p], IdleUnits: max(capacity[p]-booked[p], 0)}
		l.Utilization = math.Round(float64(booked[p])/float64(capacity[p])*100) / 100
		f.Lines = append(f.Lines, l)
		if _, dealable := unitCostCents[p]; dealable && l.IdleUnits > best.IdleUnits {
			best = l
		}
	}
	// Worth a deal when at least 20 packs' worth of a line would sit idle
	// and the line is under 70% booked.
	if best.IdleUnits < 20*dealPack || best.Utilization >= 0.7 {
		f.NotNeeded = []string{"propose_deal"}
		return f
	}
	floor := DealFloorCents(best.Product, dealPack)
	f.SuggestedDeal = &DealSuggestion{
		Product: best.Product, PackSize: dealPack, FloorCents: floor, Days: 5,
		PriceCents: (floor*5/4 + 99) / 100 * 100, // 25% over the floor, rounded up to a dollar
	}
	return f
}

// CapacityForecast shows where presses will sit idle.
func CapacityForecast(f Factory) Tool {
	return Func(domain.ToolSpec{
		Name: "capacity_forecast",
		Description: "Forecast each product line's spare press capacity over the coming days from the work already booked, " +
			"with a suggested limited-time deal for the idlest line when one is worth running.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {"horizonDays": {"type": "integer", "minimum": 3, "maximum": 28}},
		  "required": ["horizonDays"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in struct {
		HorizonDays int `json:"horizonDays"`
	}) (any, error) {
		stations, err := f.Stations(ctx)
		if err != nil {
			return nil, err
		}
		open, err := f.OpenJobs(ctx)
		if err != nil {
			return nil, err
		}
		return ForecastCapacity(stations, open, in.HorizonDays), nil
	})
}

type dealInput struct {
	Product    string `json:"product"`
	PackSize   int    `json:"packSize"`
	PriceCents int    `json:"priceCents"`
	Days       int    `json:"days"`
	Headline   string `json:"headline"`
}

// ProposeDeal schedules a limited-time deal. Prices shown to every visitor
// need a person's approval, and the margin floor is enforced here whatever
// the model proposes.
func ProposeDeal(c Commerce) Tool {
	return Func(domain.ToolSpec{
		Name:             "propose_deal",
		RequiresApproval: true,
		Description:      "Schedule a limited-time deal on a pack of one product, for up to 7 days. Must keep a 20% gross margin. Requires human approval.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {
		    "product":    {"type": "string", "enum": ["STICKERS","LABELS","MAGNETS","BUTTONS"]},
		    "packSize":   {"type": "integer", "minimum": 10, "maximum": 500},
		    "priceCents": {"type": "integer", "minimum": 100, "maximum": 100000},
		    "days":       {"type": "integer", "minimum": 1, "maximum": 7},
		    "headline":   {"type": "string", "minLength": 10, "maxLength": 200}
		  },
		  "required": ["product","packSize","priceCents","days","headline"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in dealInput) (any, error) {
		if floor := DealFloorCents(in.Product, in.PackSize); in.PriceCents < floor {
			return nil, fmt.Errorf("%s for %d %s is below the %s floor that keeps a %.0f%% margin",
				usd(in.PriceCents), in.PackSize, in.Product, usd(floor), minDealMargin*100)
		}
		id, err := c.Record(ctx, "DEAL", idempotencyKey(ctx, "deal"), in)
		if err != nil {
			return nil, err
		}
		cost := unitCostCents[in.Product] * in.PackSize
		margin := math.Round(float64(in.PriceCents-cost)/float64(in.PriceCents)*1000) / 10
		return map[string]any{"dealId": id, "status": "SCHEDULED", "days": in.Days, "marginPercent": margin}, nil
	})
}

// ---- demo tables

const jobColumns = `id, order_id, product, quantity, station_id, status, queued_at, ship_by`

func scanJob(row pgx.CollectableRow) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.OrderID, &j.Product, &j.Quantity, &j.Station, &j.Status, &j.QueuedAt, &j.ShipBy)
	return j, err
}

func (c *PGCommerce) Job(ctx context.Context, id string) (Job, error) {
	rows, err := c.pool.Query(ctx, `SELECT `+jobColumns+` FROM production_jobs WHERE id = $1`, id)
	if err != nil {
		return Job{}, err
	}
	j, err := pgx.CollectExactlyOneRow(rows, scanJob)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, fmt.Errorf("job %s: %w", id, ErrNotFound)
	}
	return j, err
}

func (c *PGCommerce) OpenJobs(ctx context.Context) ([]Job, error) {
	rows, err := c.pool.Query(ctx, `SELECT `+jobColumns+` FROM production_jobs
		WHERE status IN ('QUEUED', 'PRINTING') ORDER BY queued_at, id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanJob)
}

func (c *PGCommerce) Stations(ctx context.Context) ([]Station, error) {
	rows, err := c.pool.Query(ctx, `SELECT id, products, units_per_day FROM production_stations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Station])
}
