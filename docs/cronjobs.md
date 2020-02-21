# Primed Cronjob Schedules
This document provides an overview of the existing logic that determines primed cronjob schedules based on the schedules of incoming cronjobs. Primed cronjobs are designed to warmup the cluster X minutes before the intended cronjob kicks off. This ensures that all required nodes are pre-warmed and readily available when required. The challenging bit of this procedure is that incoming cronjob schedules need to be converted to primed cronjob schedules (original schedule minus X minutes) in order to be scheduled properly to warmup the cluster.

## Cron schedule expressions
A cron schedule consists of 5 fields separated by a space:
```
<minute> <hour> <day-of-month> <month> <day-of-week>
(0-59)   (0-23) (1-31)         (1-12)  (0-6)
```
For standard cron expressions, each field contains either an `*` (every occurrence) or a `number` shown in the range above. The cron expression `0 * * * *` runs e.g. at every hour on minute 0, while `0 2 * * *` only runs at 2AM every day.

>For more information on non-standard cron expressions and a nice playground, please use [crontab.guru](https://crontab.guru).

## Existing implementation
The existing implementation returns primed schedules for most standard and non-standard cron schedules in the  `CreatePrimerSchedule()` function in `controllers/utilities.go`.


### 1. Convert Step and Range expressions to Commas
After initial cron validation, non-standard step and range cron expressions are converted to commas in respectively `ConvertStepCronToCommaCron()` and `ConvertRangeCronToCommaCron()`. When the original cron contains step values (e.g. `0/30 * * * *`) these are converted to commas (e.g. `0,30 * * * *`) to easier determine the primed cron schedule. Similarly, ranges (e.g. `0-5 * * * *`) are converted to commas (e.g. `0,1,2,3,4,5 * * * *`).

After conversion, the cron expression follows one of the below paths, depending on whether commas are present are not

#### 2a. Cron expressions without commas
When no commas (and therefore no step/range expressions) are present, the `warmupMinutes` are deducted from the first cron field in `DeductWarmupMinutes()`. This function returns a `negativeMinutes` boolean when this subtraction resulted in a 'negative' minute which needs to be accounted for. 

Some examples:
- Incoming cron is `30 * * * *` with `10` warmupMinutes
  - Returned cron equals `20 * * * *` 
  - `negativeMinutes = false` because minutes didn't go negative after deduction
- Incoming cron is `0 * * * *` with `10` warmupMinutes
  - Returned cron equals `-10 * * * *` 
  - Above cron is invalid and 60 minutes need to be added to return: `50 * * * *` 
  - `negativeMinutes = true` because minutes went negative after deduction

Why do we need the `negativeMinutes` boolean? Because when cronjobs are hour-specific (2nd field is not '*'), the hours also need to be adjusted with `-1`. This scenario is only supported when day, month and day of the week (last 3 fields) are not specified (i.e. cronjobs ending with `x x * * *`).

#### 2b. Cron expressions with commas
In a similar fashion `warmupMinutes` are deducted for every comma-separated minute using `DeductWarmupMinutes()`. When `negativeMinutes` for one of these values equals `true`, this is currently only supported when hour, day, month and day of the week (last 4 fields) are not specified (i.e. cronjobs ending with `x * * * *`). Cron expressions with commas are therefore a little less flexible.

### 3. Primer schedule validation
As a last step, the resulting primer cron expression is parsed to validate the resulting expression. Theoretically these primed schedules should be valid, but this is an extra step to catch errors, especially when extra adding logic to this utility function.


## Testing
Several valid and invalid test schedules are defined in `controllers/utilities_test.go` and need to pass for a successful build of the code. New tests can be added by adding an extra object to the `scenario` object in the `TestPrimerScheduleString()` function with the following parameters:

```
{
    ScenarioName: "detailed name of tested scenario for debugging",
    ScenarioType: AdmissionAllowed, // or AdmissionRejected if you expect it to fail
    inputData: InputData{
        Cronjob:    "0 0 * * *", // input cronjob
        WarmupTime: 10, // input warmup minutes
    },
    expected: Expected{
        ExpectedCronjob: "50 23 * * *", // expected result or "" (empty) for expected failed result
    },
},
```

## Known issues
As mentioned before, not all cron expressions can be converted to valid primed crons, especially for non-standard expressions. Below is a list of known unsupported cron expressions:
- Can't convert special characters in cron-hour field (e.g. `0 14-16 * * *`)
- Can't use combination of range and step values in cron field (e.g. `0-15 0 * * *`)
- Can't change day, month or day-of-the-week when negativeMinutes is true and one of these is set (e.g. `0 0 * * 5`)