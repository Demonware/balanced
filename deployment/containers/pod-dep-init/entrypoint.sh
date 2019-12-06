#!/bin/sh
set -e

[ -z "$NODE_NAME" ] && echo "Need to set NODE_NAME" && exit 1;
[ -z "$LABEL_SELECTOR" ] && echo "Need to set LABEL_SELECTOR" && exit 1;
[ -z "$NAMESPACE" ] && echo "Need to set NAMESPACE" && exit 1;

while true; do
    matched_pods=$(kubectl get pods --field-selector=status.phase=Running,spec.nodeName=$NODE_NAME -l $LABEL_SELECTOR -n $NAMESPACE -ojson)
    matched_pods_len=$(echo "$matched_pods" | jq -r '.items | length')
    matched_pods_names=$(echo "$matched_pods" | jq -r '.items[] | .metadata.name')

    if [ "$matched_pods_len" -gt "0" ]; then
        echo "Matched Running Pod(s): $matched_pods_names"
        exit 0
    fi
    sleep 1
done
