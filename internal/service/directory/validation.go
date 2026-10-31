package directory

import (
	"time"

	_ "time/tzdata"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

// FieldError contains a stable input name and corrective text, never submitted
// data. The order follows the seller form so summaries remain predictable.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string { return ErrInvalid.Error() }
func (e *ValidationError) Unwrap() error { return ErrInvalid }

func weeklyMinute(value string) (int, bool) {
	if len(value) != 5 || value[2] != ':' || value[0] < '0' || value[0] > '9' || value[1] < '0' || value[1] > '9' || value[3] < '0' || value[3] > '9' || value[4] < '0' || value[4] > '9' {
		return 0, false
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	minute := int(value[3]-'0')*10 + int(value[4]-'0')
	if hour > 23 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

func validWeeklyHours(hours []db.ServiceWeeklyHour) bool {
	if len(hours) > 7 {
		return false
	}
	var seen [7]bool
	for _, hour := range hours {
		if hour.Weekday < 0 || hour.Weekday > 6 || seen[hour.Weekday] {
			return false
		}
		seen[hour.Weekday] = true
		opens, opensOK := weeklyMinute(hour.OpensAt)
		closes, closesOK := weeklyMinute(hour.ClosesAt)
		if !opensOK || !closesOK || opens >= closes {
			return false
		}
	}
	return true
}

func validateInput(id int, in Input, price money.Cents, priceErr error) error {
	e := &ValidationError{}
	check := func(ok bool, field, message string) {
		if !ok {
			e.Fields = append(e.Fields, FieldError{field, message})
		}
	}
	check(text(in.Title, 3, 120, false), "title", "Service title must contain 3–120 characters without control characters.")
	check(in.DurationMinutes >= 5 && in.DurationMinutes <= 480, "duration_minutes", "Duration must be a whole number from 5 to 480 minutes.")
	check(priceErr == nil && price >= 0 && price <= 100000000 && (in.PriceMode != "quote" || price == 0), "price", "Price must be between 0 and 1,000,000 with at most two decimal places. Use zero for quote-only pricing.")
	check(text(in.PublicArea, 2, 120, false), "public_area", "Public neighbourhood or town must contain 2–120 characters without control characters.")
	check(text(in.Description, 10, 3000, true), "description", "Description must contain 10–3000 characters; line breaks and tabs are allowed, other control characters are not.")
	check(oneOf(in.Category, "hair", "beauty", "dental", "wellness", "home", "other"), "category", "Choose one of the listed service categories.")
	check(oneOf(in.PriceMode, "fixed", "from", "quote"), "price_mode", "Choose fixed price, starting from or quote only.")
	check(oneOf(in.Currency, "KES", "USD"), "currency", "Choose KES or USD.")
	check(oneOf(in.DeliveryMode, "premises", "mobile", "remote"), "delivery_mode", "Choose at the business, provider travels to customer or remote.")
	_, timezoneErr := time.LoadLocation(in.Timezone)
	check(len(in.Timezone) <= 64 && in.Timezone != "Local" && timezoneErr == nil, "timezone", "Choose a valid IANA timezone.")
	check(oneOf(in.State, "draft", "published", "archived"), "state", "Choose draft, published or archived visibility.")
	check(validWeeklyHours(in.WeeklyHours), "weekly_hours", "Choose at most one valid opening interval per weekday; closing time must be later than opening time.")
	check(id >= 0 && ((id == 0 && in.Version == 0) || (id > 0 && in.Version >= 1)), "version", "Reload the listing form before saving; its version is missing or invalid.")
	if len(e.Fields) > 0 {
		return e
	}
	return nil
}
