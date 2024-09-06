Test the controller:
Create a Pod: kubectl run test-pod --image=busybox --command -- sleep 3600
Delete the Pod: kubectl delete pod test-pod
