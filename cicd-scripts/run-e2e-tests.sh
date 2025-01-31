# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

ROOTDIR="$(cd "$(dirname "$0")/.." ; pwd -P)"

if [[ -z "${KUBECONFIG}" ]]; then
  echo "Error: environment variable KUBECONFIG must be specified!"
  exit 1
fi

app_domain=$(oc -n openshift-ingress-operator get ingresscontrollers default -ojsonpath='{.status.domain}')
base_domain="${app_domain#apps.}"

kubeconfig_hub_path="${HOME}/.kube/kubeconfig-hub"
oc config view --raw --minify > ${kubeconfig_hub_path}

git clone --depth 1 https://github.com/open-cluster-management/observability-gitops.git

# remove the options file if it exists
rm -f resources/options.yaml

printf "options:" >> resources/options.yaml
printf "\n  hub:" >> resources/options.yaml
printf "\n    baseDomain: ${base_domain}" >> resources/options.yaml
printf "\n    grafanaURL: http://grafana-test.${app_domain}" >> resources/options.yaml
printf "\n  clusters:" >> resources/options.yaml
printf "\n    - name: cluster1" >> resources/options.yaml
printf "\n      baseDomain: ${base_domain}" >> resources/options.yaml
printf "\n      kubecontext: ${kubeconfig_hub_path}" >> resources/options.yaml

export SKIP_INSTALL_STEP=true

go get -u github.com/onsi/ginkgo/ginkgo
ginkgo -debug -trace -v ${ROOTDIR}/pkg/tests -- -options=${ROOTDIR}/resources/options.yaml -v=3

cat ${ROOTDIR}/pkg/tests/results.xml | grep failures=\"0\" | grep errors=\"0\"
if [ $? -ne 0 ]; then
    echo "Cannot pass all test cases."
    cat ${ROOTDIR}/pkg/tests/results.xml
    exit 1
fi
