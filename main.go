package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

// Record represents one row of the CSV (Date and TicketVolume).
type Record struct {
	CalledTime     time.Time
	AnsweredTime   time.Time
	HangupTime     time.Time
	EventTime      time.Time
	WaitDuration   float64
	TalkedDuration float64
}

type FlatSchedule map[string]string

func parseTime(value string) (time.Time, error) {
	return time.Parse("2006/01/02 15:04", value)
}

func getRecords(csvFilePath string) ([]Record, error) {
	file, err := os.Open(csvFilePath)
	if err != nil {
		return nil, fmt.Errorf("error opening CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ';' // Adjust if your CSV uses semicolons

	// Read header row.
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("error reading CSV header: %w", err)
	}

	// Map header column names (in lower case) to their indices.
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToLower(col)] = i
	}

	var records []Record
	for {
		row, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("error reading row: %v", err)
			continue
		}

		// Parse CalledTime (assumed to always be present)
		calledStr := strings.TrimSpace(row[colIdx["called_time"]])
		calledTime, err := parseTime(calledStr)
		if err != nil {
			log.Printf("error parsing called_time %q: %v", calledStr, err)
			continue
		}

		// Parse AnsweredTime if available.
		var answeredTime time.Time
		if idx, ok := colIdx["answered_time"]; ok {
			answeredStr := strings.TrimSpace(row[idx])
			if answeredStr != "" {
				answeredTime, err = parseTime(answeredStr)
				if err != nil {
					log.Printf("error parsing answered_time %q: %v", answeredStr, err)
				}
			}
		}

		// Parse HangupTime if available.
		var hangupTime time.Time
		if idx, ok := colIdx["hangup_time"]; ok {
			hangupStr := strings.TrimSpace(row[idx])
			if hangupStr != "" {
				hangupTime, err = parseTime(hangupStr)
				if err != nil {
					log.Printf("error parsing hangup_time %q: %v", hangupStr, err)
				}
			}
		}

		// Parse EventTime if available.
		var eventTime time.Time
		if idx, ok := colIdx["event_timestamp"]; ok {
			eventStr := strings.TrimSpace(row[idx])
			if eventStr != "" {
				eventTime, err = parseTime(eventStr)
				if err != nil {
					log.Printf("error parsing event_timestamp %q: %v", eventStr, err)
				}
			}
		}

		// Parse WaitDuration if available.
		var waitDuration float64
		if idx, ok := colIdx["wait_duration"]; ok {
			waitStr := strings.TrimSpace(row[idx])
			if waitStr != "" {
				waitDuration, err = strconv.ParseFloat(waitStr, 64)
				if err != nil {
					log.Printf("error parsing wait_duration %q: %v", waitStr, err)
				}
			}
		}

		// Parse TalkedDuration if available.
		var talkedDuration float64
		if idx, ok := colIdx["talked_duration"]; ok {
			talkedStr := strings.TrimSpace(row[idx])
			if talkedStr != "" {
				talkedDuration, err = strconv.ParseFloat(talkedStr, 64)
				if err != nil {
					log.Printf("error parsing talked_duration %q: %v", talkedStr, err)
				}
			}
		}

		// Create the record and append it.
		record := Record{
			CalledTime:     calledTime,
			AnsweredTime:   answeredTime,
			HangupTime:     hangupTime,
			EventTime:      eventTime,
			WaitDuration:   waitDuration,
			TalkedDuration: talkedDuration,
		}
		records = append(records, record)
	}

	return records, nil
}

func computeDayCounts(records []Record) map[int]int {
	counts := make(map[int]int)
	for _, rec := range records {
		day := rec.CalledTime.Day()
		counts[day]++
	}
	return counts
}

func computeThreshold(values []int, percentile float64) float64 {
	sort.Ints(values)
	index := int((percentile / 100.0) * float64(len(values)))
	if index >= len(values) {
		index = len(values) - 1
	}
	return float64(values[index])
}

func getHighVolumeDayNumbers(records []Record, percentile float64) []int {
	countsMap := computeDayCounts(records)
	var counts []int
	for _, count := range countsMap {
		counts = append(counts, count)
	}
	threshold := computeThreshold(counts, percentile)
	var highVolumeDays []int
	for day, count := range countsMap {
		if float64(count) > threshold {
			highVolumeDays = append(highVolumeDays, day)
		}
	}
	sort.Ints(highVolumeDays)
	return highVolumeDays
}

func buildPrompt(employeeNames []string, highVolumeDayNumbers []int) string {
	var dayStrs []string
	for _, d := range highVolumeDayNumbers {
		dayStrs = append(dayStrs, strconv.Itoa(d))
	}
	prompt := fmt.Sprintf(`
You are a scheduling software application. Utilizing forecasted dates that experience high ticket volumes, your job is to ensure that we have at least 20 percent more employees scheduled on those days. Your purpose is to also generate a five-week schedule in other words a monthly schedule. Work days for employees are Monday to Sunday. 

High Volume Days: %s and Employees: %s

Shifts: 
- 6 am - 3 pm which is considered an "Early Shift"
- 8 am - 5 pm which is considered a "Normal Shift"
- 11 am - 8 pm which is considered a "Late Shift"
- NOTE: a completed shift is when an employee has worked 5 days of the same shift before being assigned a new shift.

Operation Constraints **STRICT**:
- Shift coverage: Ensure each shift has at least two employees scheduled per day when possible. Ensure every day has at least two employees per shift to avoid experiencing downtime.
- Shift rotation: Ensure that each week employees are rotated between shifts. For example: Alice - Week 1 Early, Alice - Week 2 Normal, Alice - Week 3 Late, and so on.
- Off Days: Try your hardest to give employees at least two weekends Saturday and Sunday off at least twice in that five-week schedule. Try your hardest to ensure that employees get two rest days before the start of a new shift if possible. Maximum of two days off per week.
- Scheduling: I recommend grouping employees as evenly as possible and rotating the shifts between those groups.
- Hours: On a weekly, employees can only work 45 hours per week, and in a month they can only work 225. Employees are also to be scheduled every week.

Do not return any extra text. Only generate the five-week schedule. The desired output should just be a JSON array of objects and each object represents one employee schedule such as: 
{"Week": "Week 1", "Employee": "Alice", "Monday (1st March)": "Early", "Tuesday (2nd March)": "Normal", "Wednesday (3rd March)": "Late", "Thursday (4th March)": "Off", "Friday (5th March)": "Early", "Saturday (6th March)": "Off", "Sunday (7th March)": "Normal"}

If constraints cannot be met please do not proceed with providing an output. 
`, strings.Join(dayStrs, ", "), strings.Join(employeeNames, ", "))
	return prompt
}

func callChatGPT(prompt string) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", errors.New("OPENAI_API_KEY not set")
	}

	client := openai.NewClient(apiKey)
	ctx := context.Background()

	req := openai.ChatCompletionRequest{
		Model:       openai.GPT4oMini,
		Temperature: 0.5,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleAssistant, Content: prompt},
		},
	}

	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("ChatCompletion error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from API")
	}

	return resp.Choices[0].Message.Content, nil
}

func groupObjectsByWeek(jsonStr string) (map[string][]FlatSchedule, error) {
	var entries []FlatSchedule
	if err := json.Unmarshal([]byte(jsonStr), &entries); err != nil {
		return nil, fmt.Errorf("error unmarshaling JSON array: %w", err)
	}

	weeks := make(map[string][]FlatSchedule)
	for _, entry := range entries {
		weekKey, ok := entry["Week"]
		if !ok {
			continue
		}
		weeks[weekKey] = append(weeks[weekKey], entry)
	}
	return weeks, nil
}

func extractDayNumber(key string) int {
	parts := strings.Split(key, " ")
	if len(parts) < 2 {
		return 0
	}

	numStr := ""
	for _, r := range parts[1] {
		if r >= '0' && r <= '9' {
			numStr += string(r)
		}
	}
	day, err := strconv.Atoi(numStr)
	if err != nil {
		return 0
	}
	return day
}

func buildHeaderForWeek(objs []FlatSchedule) []string {
	keySet := make(map[string]struct{})
	for _, obj := range objs {
		for key := range obj {
			keySet[key] = struct{}{}
		}
	}

	// Start with fixed keys.
	header := []string{"Week", "Employee"}

	var dayKeys, otherKeys []string
	for key := range keySet {
		if key == "Week" || key == "Employee" {
			continue
		}
		if strings.Contains(key, "(") {
			dayKeys = append(dayKeys, key)
		} else {
			otherKeys = append(otherKeys, key)
		}
	}

	// Sort day keys by the numeric day extracted from the key.
	sort.SliceStable(dayKeys, func(i, j int) bool {
		return extractDayNumber(dayKeys[i]) < extractDayNumber(dayKeys[j])
	})
	sort.Strings(otherKeys)

	header = append(header, dayKeys...)
	header = append(header, otherKeys...)
	return header
}

func buildTableForWeek(header []string, objs []FlatSchedule) [][]string {
	table := [][]string{header}
	for _, obj := range objs {
		row := make([]string, len(header))
		for i, key := range header {
			if val, ok := obj[key]; ok {
				row[i] = val
			} else {
				row[i] = ""
			}
		}
		table = append(table, row)
	}
	return table
}

func main() {
	csvFilePath := "" // Please set this.
	records, err := getRecords(csvFilePath)
	if err != nil {
		log.Fatalf("Error processing CSV: %v", err)
	}
	log.Printf("Processed %d records.\n", len(records))

	// Compute high-volume day numbers.
	highVolumeDays := getHighVolumeDayNumbers(records, 75)
	log.Printf("High volume day numbers: %v", highVolumeDays)

	// Example employee names.
	employeeNames := []string{"Alice", "Bob", "Charlie", "David", "Eva", "Frank", "Grace", "Hannah", "Mbuso"}

	// Build the scheduling prompt.
	prompt := buildPrompt(employeeNames, highVolumeDays)

	// Call ChatGPT (replace this with your actual API call).
	response, err := callChatGPT(prompt)
	if err != nil {
		log.Fatalf("Error calling ChatGPT: %v", err)
	}
	fmt.Println("ChatGPT Response:", response)

	// --- Clean and extract the JSON part ---
	startIndex := strings.IndexAny(response, "[{")
	if startIndex == -1 {
		log.Fatalf("No JSON array or object found in the response")
	}
	jsonPart := strings.Trim(response[startIndex:], " \n`")

	// Group objects by week.
	weeks, err := groupObjectsByWeek(jsonPart)
	if err != nil {
		log.Fatalf("Error grouping objects by week: %v", err)
	}

	// For each week, build a header and table, then write a CSV file.
	for week, objs := range weeks {
		header := buildHeaderForWeek(objs)
		table := buildTableForWeek(header, objs)
		filename := fmt.Sprintf("generated_schedule_%s.csv", strings.ReplaceAll(week, " ", ""))
		csvFile, err := os.Create(filename)
		if err != nil {
			log.Fatalf("Error creating CSV file %s: %v", filename, err)
		}
		writer := csv.NewWriter(csvFile)
		if err := writer.WriteAll(table); err != nil {
			log.Fatalf("Error writing CSV data to %s: %v", filename, err)
		}
		writer.Flush()
		csvFile.Close()
		log.Printf("Schedule for %s saved to %s", week, filename)
	}
}
