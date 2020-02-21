from croniter import croniter
from datetime import datetime
from kubernetes import client, config
import time
import os

def get_pod_creation_date(podName, podNamespace):
    try:
        config.load_incluster_config()
    except: 
        config.load_kube_config()

    v1 = client.CoreV1Api()
    podStatus = v1.read_namespaced_pod_status(name=podName, namespace=podNamespace)
    return podStatus.metadata.creation_timestamp

def wait_on_cron_schedule(creationDate, schedule):
    if schedule:
        if croniter.is_valid(schedule):
            cron = croniter(schedule, creationDate)
            nextdate = cron.get_next(datetime)

            while True:
                now = datetime.now().astimezone() # needs to be tz-aware to compare

                if now >= nextdate:
                    print("finally reached!")
                    break

                print("current time: " + now.strftime("%m/%d/%Y, %H:%M:%S"))    
                print("didn't reach " + nextdate.strftime("%m/%d/%Y, %H:%M:%S"))

                time.sleep(5)
        else:
            print("invalid cron schedule")
    else:
        print("no cron schedule passed via env variables")

if __name__ == '__main__':
    creationDate = get_pod_creation_date(os.environ.get('HOSTNAME'), os.environ.get('NAMESPACE'))
    wait_on_cron_schedule(creationDate, os.environ.get('CRONJOB_SCHEDULE'))
