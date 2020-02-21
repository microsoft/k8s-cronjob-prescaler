# Debugging

## Debug locally
To debug in Kind and hit breakpoints on your laptop, follow these steps:

1. Ensure you have the operator [deployed locally](../readme#deploying%20locally).
2. Set a breakpoint in `./controllers/prescaledcronjob_controller.go` and hit `F5`
3. Create a new prescaledcronjob object: `make recreate-sample-psccron`
4. all being well... your breakpoint will get hit.

## Checking the logs
The Operator records debug information with the logs for the controller manager. To view them:
- run `kubectl get pods -A`
- copy the name of your controller manager, for example: `pod/psc-controller-manager-6544fc674f-nl5d2`
- run `kubectl logs <pod name> -n psc-system manager` (so in our example: `kubectl logs pod/psc-controller-manager-6544fc674f-nl5d2 -n psc-system manager`)

## Checking object events
The Operator records events on the `PreScaledCronJob` objects as they occur. To view them:
- run `kubectl describe prescaledcronjobs <your prescaledcronjob name here> -n psc-system`
- you will be shown all events that have taken place related to the `prescaledcronjob` object you created