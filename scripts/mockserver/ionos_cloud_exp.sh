#!/usr/bin/bash

# function with parameter filename
function create_expectation {
    curl -s -X PUT $expectationurl -d @scripts/mockserver/$1 > /dev/null 2>&1
}

function reset() {
    curl -s -X PUT ${mockserver_base_url}/reset > /dev/null 2>&1
}

mockserver_base_url="http://localhost:1080/mockserver"
expectationspath="expectation"
expectationurl="${mockserver_base_url}/${expectationspath}"

reset
create_expectation "ionos_cloud_get_zones_exp.json"
create_expectation "ionos_cloud_get_records_exp.json"

