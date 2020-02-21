package controllers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/robfig/cron/v3"
)

// GetPrimerSchedule tries to parse (an optional) primerSchedule and otherwise manually creates the primerSchedule
func GetPrimerSchedule(scheduleSpec string, warmupMinutes int, primerSchedule string) (string, error) {
	if primerSchedule != "" {
		// parse primer schedule
		_, err := cron.ParseStandard(primerSchedule)
		if err == nil {
			return primerSchedule, err
		}

		return "", fmt.Errorf("primerSchedule provided is invalid: %v", err)
	}

	// no pre-defined primer schedule or couldn't parse it, creating it with warmup minutes instead
	return CreatePrimerSchedule(scheduleSpec, warmupMinutes)
}

// CreatePrimerSchedule deducts the warmup time from the original cronjob schedule and creates a primed cronjob schedule
func CreatePrimerSchedule(scheduleSpec string, warmupMinutes int) (string, error) {
	// bool that will be set to true if the minute expression goes < 0 after deduction
	// therefore the hour expression will also need adjustments if not set to *
	negativeMinutes := false

	// parse schedule
	_, err := cron.ParseStandard(scheduleSpec)
	if err != nil {
		return "", fmt.Errorf("scheduleSpec provided is invalid: %v", err)
	}

	cronArray := strings.Split(scheduleSpec, " ")

	// validate cronjob
	if cronArray[0] == "*" {
		err = fmt.Errorf("Can't create primer schedule on something that runs every minute")
		return "", err
	}

	// convert cron expressions with step (/) to commas
	if strings.Contains(cronArray[0], "/") {
		cronArray, err = convertStepCronToCommaCron(cronArray)

		if err != nil {
			return "", err
		}
	}

	// convert cron expressions with range (-) to commas
	if strings.Contains(cronArray[0], "-") {
		cronArray, err = convertRangeCronToCommaCron(cronArray)

		if err != nil {
			return "", err
		}
	}

	if strings.Contains(cronArray[0], ",") {
		commaValues := strings.Split(cronArray[0], ",")

		for i, s := range commaValues {
			commaValues[i], negativeMinutes, err = deductWarmupMinutes(s, warmupMinutes)

			if err != nil {
				return "", err
			}

			if negativeMinutes && cronArray[1] != "*" {
				return "", fmt.Errorf("Can't adjust hour for minute expression with multiple values")
			}
		}

		cronArray[0] = strings.Join(commaValues, ",")

	} else {
		cronArray[0], negativeMinutes, err = deductWarmupMinutes(cronArray[0], warmupMinutes)
		if err != nil {
			return "", err
		}

		if negativeMinutes && cronArray[1] != "*" {
			hourCron, minErr := strconv.Atoi(cronArray[1])
			if minErr != nil {
				return "", fmt.Errorf("Can't adjust special characters in cron-hour argument")
			}

			// adjust hour param (if not set to *)
			cronArray[1] = strconv.Itoa(hourCron - 1)

			// only allow changing a 'negative' hour when the cron expression is not day-specific
			if cronArray[1] == "-1" && cronArray[2] == "*" && cronArray[3] == "*" && cronArray[4] == "*" {
				cronArray[1] = "23"
			} else if cronArray[1] == "-1" {
				// cronjobs that run on midnight on a specific day are not supported
				return "", fmt.Errorf("Unsupported cron, can't create primer cronjob with this expression")
			}
		} else if negativeMinutes &&
			(cronArray[2] != "*" || cronArray[3] != "*" || cronArray[4] != "*") {
			// cronjobs that run on midnight on a specific day are not supported
			return "", fmt.Errorf("Unsupported cron, can't create primer cronjob with this expression")
		}
	}

	// parse primer schedule
	primerSchedule := strings.Join(cronArray, " ")
	_, err = cron.ParseStandard(primerSchedule)
	if err != nil {
		return "", err
	}

	return primerSchedule, nil
}

func convertStepCronToCommaCron(cronArray []string) ([]string, error) {
	splitStepVal := strings.Split(cronArray[0], "/")

	// convert */x to 0/x since it's the same but easier to work with
	if splitStepVal[0] == "*" {
		splitStepVal[0] = "0"
	}

	startVal, err1 := strconv.Atoi(splitStepVal[0])
	stepVal, err2 := strconv.Atoi(splitStepVal[1])
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("Can't break up step values")
	}

	cronArray[0] = splitStepVal[0] + ","

	for startVal+stepVal < 60 {
		startVal += stepVal
		cronArray[0] += strconv.Itoa(startVal) + ","
	}

	// remove trailing comma
	cronArray[0] = strings.TrimSuffix(cronArray[0], ",")

	return cronArray, nil
}

func convertRangeCronToCommaCron(cronArray []string) ([]string, error) {
	rangeVal := strings.Split(cronArray[0], "-")
	startVal, err1 := strconv.Atoi(rangeVal[0])
	endVal, err2 := strconv.Atoi(rangeVal[1])
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("Can't break up range values")
	}

	cronArray[0] = rangeVal[0] + ","

	for startVal+1 <= endVal {
		startVal++
		cronArray[0] += strconv.Itoa(startVal) + ","
	}

	// remove trailing comma
	cronArray[0] = strings.TrimSuffix(cronArray[0], ",")

	return cronArray, nil
}

func deductWarmupMinutes(minuteVal string, warmupMinutes int) (string, bool, error) {
	negativeMinutes := false
	minuteCron, err := strconv.Atoi(minuteVal)
	if err != nil {
		return "", negativeMinutes, fmt.Errorf("Can't parse minute value to int")
	}

	minuteVal = strconv.Itoa(minuteCron - warmupMinutes)

	// when cronjob-minute param minus warmupTime min is smaller than 0
	if (minuteCron - warmupMinutes) < 0 {
		// add 60 (so that e.g. -5 becomes 55)
		minuteVal = strconv.Itoa(minuteCron - warmupMinutes + 60)
		negativeMinutes = true
	}

	return minuteVal, negativeMinutes, nil
}
