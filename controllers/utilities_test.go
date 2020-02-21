package controllers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	AdmissionAllowed  = "AdmissionAllowed"
	AdmissionRejected = "AdmissoinRejected"
)

type InputData struct {
	Cronjob    string
	WarmupTime int
}

type Expected struct {
	AdmissionAllowed bool
	ExpectedCronjob  string
}

type TestData struct {
	ScenarioName string
	ScenarioType string
	inputData    InputData
	expected     Expected
}

func assertReviewResult(testData TestData, t *testing.T) {
	actualResult, err := CreatePrimerSchedule(testData.inputData.Cronjob, testData.inputData.WarmupTime)

	actualAdmissionAllowed := true
	if err != nil {
		actualAdmissionAllowed = false
	}

	require.Equal(t, testData.expected.AdmissionAllowed, actualAdmissionAllowed)
	require.Equal(t, testData.expected.ExpectedCronjob, actualResult)
}

func TestCreatePrimerSchedule(t *testing.T) {

	scenarios := []TestData{
		{
			ScenarioName: "valid basic cron that runs on specific minute",
			ScenarioType: AdmissionAllowed,
			inputData: InputData{
				Cronjob:    "30 * * 10 *",
				WarmupTime: 5,
			},
			expected: Expected{
				ExpectedCronjob: "25 * * 10 *",
			},
		},
		{
			ScenarioName: "valid primer cron that needs to adjust the hours",
			ScenarioType: AdmissionAllowed,
			inputData: InputData{
				Cronjob:    "0 0 * * *",
				WarmupTime: 10,
			},
			expected: Expected{
				ExpectedCronjob: "50 23 * * *",
			},
		},
		{
			ScenarioName: "valid cron with step values",
			ScenarioType: AdmissionAllowed,
			inputData: InputData{
				Cronjob:    "0/15 * 1 * *",
				WarmupTime: 10,
			},
			expected: Expected{
				ExpectedCronjob: "50,5,20,35 * 1 * *",
			},
		},
		{
			ScenarioName: "valid cron with hour ranges",
			ScenarioType: AdmissionAllowed,
			inputData: InputData{
				Cronjob:    "30 14-16 * * *",
				WarmupTime: 10,
			},
			expected: Expected{
				ExpectedCronjob: "20 14-16 * * *",
			},
		},
		{
			ScenarioName: "valid cron with *-defined step values",
			ScenarioType: AdmissionAllowed,
			inputData: InputData{
				Cronjob:    "*/30 * 1 * *",
				WarmupTime: 10,
			},
			expected: Expected{
				ExpectedCronjob: "50,20 * 1 * *",
			},
		},
		{
			ScenarioName: "valid complicated cron with unaffected hour/day params",
			ScenarioType: AdmissionAllowed,
			inputData: InputData{
				Cronjob:    "5 * */12 * 1,2",
				WarmupTime: 5,
			},
			expected: Expected{
				ExpectedCronjob: "0 * */12 * 1,2",
			},
		},
		{
			ScenarioName: "valid cron with non-zero step value minutes",
			ScenarioType: AdmissionAllowed,
			inputData: InputData{
				Cronjob:    "15/30 5 * * *",
				WarmupTime: 5,
			},
			expected: Expected{
				ExpectedCronjob: "10,40 5 * * *",
			},
		},
		{
			ScenarioName: "valid cron with a minute range",
			ScenarioType: AdmissionAllowed,
			inputData: InputData{
				Cronjob:    "15-20 12 * * 5",
				WarmupTime: 5,
			},
			expected: Expected{
				ExpectedCronjob: "10,11,12,13,14,15 12 * * 5",
			},
		},
		{
			ScenarioName: "valid cron with comma values",
			ScenarioType: AdmissionAllowed,
			inputData: InputData{
				Cronjob:    "5,12,48,56 * * * 5",
				WarmupTime: 10,
			},
			expected: Expected{
				ExpectedCronjob: "55,2,38,46 * * * 5",
			},
		},
		{
			ScenarioName: "invalid cron (every minute)",
			ScenarioType: AdmissionRejected,
			inputData: InputData{
				Cronjob:    "* 0 * * *",
				WarmupTime: 5,
			},
			expected: Expected{
				ExpectedCronjob: "",
			},
		},
		{
			ScenarioName: "invalid cron (6 arguments instead of 5)",
			ScenarioType: AdmissionRejected,
			inputData: InputData{
				Cronjob:    "* 0 * * * *",
				WarmupTime: 5,
			},
			expected: Expected{
				ExpectedCronjob: "",
			},
		},
		{
			ScenarioName: "can't use combination of ranges and step values",
			ScenarioType: AdmissionRejected,
			inputData: InputData{
				Cronjob:    "15-17,0/30 * * * *",
				WarmupTime: 5,
			},
			expected: Expected{
				ExpectedCronjob: "",
			},
		},
		{
			ScenarioName: "can't convert special characters in cron-hour argument",
			ScenarioType: AdmissionRejected,
			inputData: InputData{
				Cronjob:    "0 14-16 * * *",
				WarmupTime: 10,
			},
			expected: Expected{
				ExpectedCronjob: "",
			},
		},
		{
			ScenarioName: "expected cronjob unable to compute due to overlap over multiple days",
			ScenarioType: AdmissionRejected,
			inputData: InputData{
				Cronjob:    "0-15 0 * * *",
				WarmupTime: 10,
			},
			expected: Expected{
				ExpectedCronjob: "",
			},
		},
		{
			ScenarioName: "invalid, expected cron needs to change day-of-the-week",
			ScenarioType: AdmissionRejected,
			inputData: InputData{
				Cronjob:    "0 0 * * 5",
				WarmupTime: 10,
			},
			expected: Expected{
				ExpectedCronjob: "",
			},
		},
		{
			ScenarioName: "invalid, expected cron needs to change day-of-the-week (no hour)",
			ScenarioType: AdmissionRejected,
			inputData: InputData{
				Cronjob:    "0 * * * 5",
				WarmupTime: 10,
			},
			expected: Expected{
				ExpectedCronjob: "",
			},
		},
	}

	for _, testData := range scenarios {
		switch testData.ScenarioType {
		case AdmissionAllowed:
			testData.expected.AdmissionAllowed = true
		case AdmissionRejected:
			testData.expected.AdmissionAllowed = false
		}

		t.Run(fmt.Sprintf("[%s]:%s", testData.ScenarioName, testData.inputData.Cronjob), func(t *testing.T) {
			assertReviewResult(testData, t)
		})
	}
}

func TestGetPrimerSchedule_ValidPrimerSchedule_Returns_PrimerSchedule(t *testing.T) {
	schedule := "30 * 15 * *"
	actualResult, err := GetPrimerSchedule("* * * * *", 10, schedule)

	if assert.NoError(t, err) {
		require.Equal(t, schedule, actualResult)
	}
}

func TestGetPrimerSchedule_InvalidPrimerSchedule_Returns_Error(t *testing.T) {
	schedule := "wibble"
	_, err := GetPrimerSchedule("* * * * *", 10, schedule)

	assert.Error(t, err)
}

func TestGetPrimerSchedule_NoPrimerSchedule_InvalidSchedule_Returns_Error(t *testing.T) {
	schedule := "wibble"
	_, err := GetPrimerSchedule(schedule, 10, "")

	assert.Error(t, err)
}
