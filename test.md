# Migration Workflow

There are multiple steps required to trigger the migration and the composition of OpenShift clusters may vary. The below guide provides instructions for converting and migrating different object types from OpenShift to Kubernetes (EKS in this case). All the steps in the document are just to provide an overview on how the objects differ between Openshift and Kubernetes.


# 1. Converting OpenShift Projects to Kubernetes Namespaces

[OpenShift projects](https://docs.openshift.com/container-platform/4.7/rest_api/project_apis/project-project-openshift-io-v1.html) are equvalent to Kubernetes Namespaces. Technically, OpenShift Project and Kubernetes Namespace are basically the same: A Project is a Kubernetes namespace with additional annotations (an functionality) to provide multi tenancy.

If you’re deploying software on OpenShift you’ll basically use the project exactly the same way as a Kubernetes namespace, except a normal user can be prevented from creating their own projects, requiring a cluster administrator to do that.

A good example would be network policies that close your project for external traffic so that is isolated and secure by default – if you want to permit some kind of traffic you would do so by creating additional policies explicitly. In a similar way, you could provide default quotas or LimitRange objects and make your new projects pre-configured according to your organization rules.

### Generating Kubernetes namespace for each OpenShift Project.

If you are migrating individual projects (recommended), here is a typical script to convert an OpenShift Project to Kubernetes namespace. Substitute the variable names accordingly before running.

```
PROJECT_NAME=<<yourprojectname>>
oc get project $PROJECT_NAME -o yaml | \
yq e '.apiVersion |= "v1"' - \
| yq e '.kind |= "Namespace"' - \
| yq e 'del(.metadata.creationTimestamp)' - \
| yq e 'del(.metadata.annotations.*)' - \
| yq e 'del(.metadata.managedFields)' - \
| yq e 'del(.metadata.labels)' - \
| yq e 'del(.metadata.resourceVersion)' - \
| yq e 'del(.metadata.selfLink)' - \
| yq e 'del(.metadata.uid)' - \
| yq e 'del(.status)' -
```

### Running across the entire OpenShift Cluster

If you are migrating the whole cluster, you can generate namespace yamls for all the project running user workloads. 

In a typical openshift cluster projects are used for cluster services as well. All projects beginning with `kube-` and `openshift-` are cluster services. In addition, there may be other projects like `istio-system`,  `dlx-2`, `dlx-1` etc, that run other services. While we are applying filters to not migrate such projects, depending on what is running on the cluster you may need additional filtering or you may need to remove any namespaces that should not be migrated.

Projects can be filtered by editing the PROJECT_FILTERS variable in the script below. The following script will show you list of projects. 

```
PROJECT_FILTERS="^openshift-\|^kube-\|^istio-system\|^dlx-"
for i in $(oc get projects -o jsonpath='{.items[*].metadata.name}'); do 
  if grep -v "$PROJECT_FILTERS" <<< $i ; then 
     echo $i 
  fi 
done
```

Based on what you installed on your cluster, you may have other openshift projects that you don't want to migrate. Verify the project listed to see if you want to filter any more. Edit the PROJECT_FILTERS as required and re-run.

Run the following command to generate kubernetes namespace configurations for the OpenShift projects. These namespaces will be saved into `clusterconfigs/namespaces` folder.

```
mkdir -p clusterconfigs/namespaces
PROJECT_FILTERS="^openshift-\|^kube-\|^istio-system\|^dlx-"
for i in $(oc get projects -o jsonpath='{.items[*].metadata.name}'); do 
if grep -v "$PROJECT_FILTERS" <<< $i ; then 
    echo "Exporting Project: " $i; \
    mkdir -p clusterconfigs/namespaces/$i; \
    oc get project $i -o yaml | \
    yq e '.apiVersion |= "v1"' - \
    | yq e '.kind |= "Namespace"' - \
    | yq e 'del(.metadata.creationTimestamp)' - \
    | yq e 'del(.metadata.annotations.*)' - \
    | yq e 'del(.metadata.managedFields)' - \
    | yq e 'del(.metadata.labels)' - \
    | yq e 'del(.metadata.resourceVersion)' - \
    | yq e 'del(.metadata.selfLink)' - \
    | yq e 'del(.metadata.uid)' - \
    | yq e 'del(.status)' -  \
    > clusterconfigs/namespaces/$i/namespace.yaml
fi
done
```
List all the namespace yamls generated in `clusterconfigs` by running `ls clusterconfigs/namespaces/NAMESPACE`
Verify each namespace file to make sure it is in the format you expect.

As an example:

```
$ cat clusterconfigs/namespaces/dlx-1/namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  annotations: {}
  name: dlx-1
spec:
  finalizers:
    - kubernetes
```

# 2. Migrating ClusterResourceQuotas

[Cluster Resource Quotas](https://docs.openshift.com/container-platform/4.7/rest_api/schedule_and_quota_apis/clusterresourcequota-quota-openshift-io-v1.html) are OpenShift specific resources that are not applicable as-is on a Kubernetes cluster. These aggregate quotas at a multiple namespace level. To apply quotas to a kubernetes cluster, you can convert these into individual namespace level resource quotas. You will have to manually decide on how much quota to allocate for individual namespaces. 

### Checking the values of individual ClusterResourceQuotas

You can get the list of ClusterResourceQuotas by running

```
oc get clusterresourcequotas
```

Removing all extraneous values, if you want to list a specific cluster resource quotas, you can check it by running:

```
CLUSTER_QUOTA_NAME=<<clusterquotaname>>
oc get clusterresourcequota $CLUSTER_QUOTA_NAME -o yaml | \
yq e 'del(.metadata.creationTimestamp)' - \
| yq e 'del(.metadata.generation)' - \
| yq e 'del(.metadata.managedFields)' - \
| yq e 'del(.metadata.annotations.*)' - \
| yq e 'del(.metadata.labels)' - \
| yq e 'del(.metadata.resourceVersion)' - \
| yq e 'del(.metadata.selfLink)' - \
| yq e 'del(.metadata.uid)' - 

```


### Generating Resource Quota Templates from ClusterResourceQuotas

The following script will generate two files for each ClusterResourceQuota.
1. Original ClusterResourceQuota that you can use to decide the namespaces this quota applies to. This is a manual decision.
2. A template for Kubernetes ResourceQuota. The quota values are retained at the cluster resource quota level. You can copy and edit this file to split up the ClusterResourceQuotas into ResourceQuotas for specific namespaces. This requires a little manual editing once you decide quota for each namespace.

The files will be saved in the folder `clusterconfigs/to-review/cluster-resource-quotas`

```
mkdir -p clusterconfigs/to-review/cluster-resource-quotas
for i in $(oc get clusterresourcequota  -o jsonpath='{.items[*].metadata.name}'); do \
echo "Exporting Cluster Resource Quota:" $i; \
oc get clusterresourcequota $i -o yaml | \
yq e 'del(.metadata.creationTimestamp)' - \
| yq e 'del(.metadata.generation)' - \
| yq e 'del(.metadata.managedFields)' - \
| yq e 'del(.metadata.annotations.*)' - \
| yq e 'del(.metadata.labels)' - \
| yq e 'del(.metadata.resourceVersion)' - \
| yq e 'del(.metadata.selfLink)' - \
| yq e 'del(.metadata.uid)' - > clusterconfigs/to-review/cluster-resource-quotas/$i.original; \
oc get clusterresourcequota $i -o yaml | \
yq e '.apiVersion |= "v1"' - \
| yq e '.kind |= "ResourceQuota"' - \
| yq e 'del(.metadata.creationTimestamp)' - \
| yq e 'del(.metadata.generation)' - \
| yq e 'del(.metadata.managedFields)' - \
| yq e 'del(.metadata.annotations.*)' - \
| yq e 'del(.metadata.labels)' - \
| yq e 'del(.metadata.resourceVersion)' - \
| yq e 'del(.metadata.selfLink)' - \
| yq e 'del(.metadata.uid)' - \
| yq e 'del(.spec.selector)' - \
| yq e '.metadata.namespace |= "CHANGEME"' - > clusterconfigs/to-review/cluster-resource-quotas/$i.yaml
done
```

The above command will generate two files for each ClusterResourceQuota as shown below:

```
$ cat clusterconfigs/to-review/cluster-resource-quotas/for-name.original
apiVersion: quota.openshift.io/v1
kind: ClusterResourceQuota
metadata:
  name: for-name
spec:
  quota:
    hard:
      pods: "10"
      secrets: "20"
  selector:
    annotations: null
    labels:
      matchLabels:
        name: frontend
```

```
$ cat clusterconfigs/to-review/cluster-resource-quotas/for-name.yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: for-name
  namespace: CHANGEME
spec:
  quota:
    hard:
      pods: "10"
      secrets: "20"
```

Note the selection criteria in the original file. You can use that to determine the openshift projects impacted by the ClusterResourceQuota. You may have to split this quota among multiple namespaces in the target kubernetes cluster.

Once you decide the specific ResourceQuotas to apply to individual namespaces, copy the second file as `clusterconfigs/namespaces/NAMESPACE/quota.yaml` and change the quota values, the value for namespace (`namespace: CHANGEME`) and save the file. This changed quota would be applied to the target kubernetes cluster.


# 3. Exporting NetNameSpaces
[NetNamespace](https://docs.openshift.com/container-platform/4.7/rest_api/network_apis/netnamespace-network-openshift-io-v1.html)s are OpenShift resources used for namespace level isolation.  In this section we will export these files. But these cannot be applied to the target cluster. The target Network Policies have to be manually configured.

### Listing individual Netnamespaces

List netnamespaces on the OpenShift cluster by running:

```
oc get netnamespaces
```

To filter netnamespaces for specific openshift projects, you an apply project filters like before.

```
PROJECT_FILTERS="^openshift-\|^kube-\|^istio-system\|^knative-"
for i in $(oc get netnamespaces -o jsonpath='{.items[*].metadata.name}'); do 
  if grep -v "$PROJECT_FILTERS" <<< $i ; then 
     echo $i 
  fi 
done
```

### Export Netnamespaces

Retrieve the list of NetNamespaces for the openshift projects of interest. These will be saved into `clusterconfigs/to-review/net-namespaces` folder for future reference.

```
PROJECT_FILTERS="^openshift-\|^kube-\|^istio-system\|^knative-"
mkdir -p clusterconfigs/to-review/net-namespaces
for i in $(oc get netnamespaces -o jsonpath='{.items[*].metadata.name}'); do 
if grep -v "$PROJECT_FILTERS" <<< $i ; then 
    echo "Exporting NetNamespace: " $i; \
    oc get netnamespaces $i -o yaml \
    | yq e 'del(.metadata.creationTimestamp)' - \
    | yq e 'del(.metadata.resourceVersion)' - \
    | yq e 'del(.metadata.selfLink)' - \
    | yq e 'del(.metadata.uid)' - \
    | yq e 'del(.status)' -  \
    > clusterconfigs/to-review/net-namespaces/$i.yaml
fi
done
```

# 4. Migrating Project Specific Resource Quotas

[ResourceQuotas](https://kubernetes.io/docs/concepts/policy/resource-quotas/) are kubernetes resources that can be applied from the source cluster to the target cluster. No changes are necessary.

### Checking values of individual ResourceQuotas

You can use this section if you are handling one resourcequota at a time.

To list ResourceQuotas in a project, run 

```
PROJECT_NAME=<<projectname>>
oc get resourcequota -n $PROJECT_NAME
```

To get a specific ResourceQuota within an OpenShift project

```
PROJECT_NAME=<<projectname>>
RQ_NAME=<<resourcequotaname>>
oc get resourcequota $RQ_NAME -n $PROJECT_NAME -o yaml \
    | yq e 'del(.metadata.creationTimestamp)' - \
    | yq e 'del(.metadata.resourceVersion)' - \
    | yq e 'del(.metadata.selfLink)' - \
    | yq e 'del(.metadata.uid)' - \
    | yq e 'del(.status)' -  \
    | yq e 'del(.metadata.managedFields)' - \
    | yq e 'del(.metadata.annotations)' - \
    | yq e 'del(.metadata.manager)' - \
    | yq e 'del(.metadata.operation)' - \
    | yq e 'del(.metadata.time)' - 

```

### Export all project level ResourceQuotas

To export ResourceQuotas across all the selected set of namespaces in the `clusterconfigs/namespaces` folder , run the following script. This will save the ResourceQuotas in `clusterconfigs/namespaces/NAMESPACE` folder, each with a unique name.

```
for ns in $(ls clusterconfigs/namespaces); do 
    for i in $(oc get resourcequotas -n $ns -o jsonpath='{.items[*].metadata.name}'); do
     echo "Exporting resource quotas" $i "for namespace" $ns; \
     mkdir -p clusterconfigs/namespaces/$ns
     oc get resourcequota $i -n $ns -o yaml \
        | yq e 'del(.metadata.creationTimestamp)' - \
        | yq e 'del(.metadata.resourceVersion)' - \
        | yq e 'del(.metadata.selfLink)' - \
        | yq e 'del(.metadata.uid)' - \
        | yq e 'del(.status)' -  \
        | yq e 'del(.metadata.managedFields)' - \
        | yq e 'del(.metadata.annotations)' - \
        | yq e 'del(.metadata.manager)' - \
        | yq e 'del(.metadata.operation)' - \
        | yq e 'del(.metadata.time)' -  > clusterconfigs/namespaces/$ns/$i-quota.yaml      
    done; 
done
```
  
# 5. Migrate Cluster Roles and Cluster Role Bindings

[ClusterRoles and ClusterRoleBindings](https://kubernetes.io/docs/reference/access-authn-authz/rbac/) are k8s resources that can be applied to the target cluster. This section exports the cluster roles and corresponding cluster role bindings. However, you may not want all the cluster roles and rolebindings on the target cluster. So while the scripts generate the files, you can manually filter out the ones needed and apply the ones you decide.

### Export Cluster Roles

To get a list of cluster roles run
```
oc get clusterroles
```

To render a specific cluster role removing extraneous information

```
CLUSTER_ROLE_NAME=<<clusterrolename>>
oc get clusterrole $CLUSTER_ROLE_NAME -o yaml | \
yq e 'del(.metadata.creationTimestamp)' - \
| yq e 'del(.metadata.resourceVersion)' - \
| yq e 'del(.metadata.managedFields)' - \
| yq e 'del(.metadata.selfLink)' - \
| yq e 'del(.metadata.uid)' - 
```

Depending on what is installed on the OpenShift cluster, there may be many cluster roles that may not be required to be ported to the target cluster. So you may have to filter out the ones that are not needed from the list generated by the following command. The following command exports cluster roles to `clusterconfigs/cluster-roles` folder:

```
mkdir -p clusterconfigs/cluster/cluster-roles
for i in $(oc get clusterroles  -o jsonpath='{.items[*].metadata.name}'); do \
echo "Exporting ClusterRole: " $i
oc get clusterrole $i -o yaml | \
yq e 'del(.metadata.creationTimestamp)' - \
| yq e 'del(.metadata.resourceVersion)' - \
| yq e 'del(.metadata.managedFields)' - \
| yq e 'del(.metadata.selfLink)' - \
| yq e 'del(.metadata.uid)' - > clusterconfigs/cluster/cluster-roles/$i.yaml; \
done
```

Review the list of cluster roles in the `projectconfigs/cluster-roles` folder and delete the manifests for the roles that should not be exported to the target cluster.

### Export Cluster Role Bindings

If you are dealing with individual cluster role binding migration, you can list the clusterrolebindings using `oc get clusterrolebindings` and then get the individual clusterrolebinding manifest by running:

```
CLUSTER_ROLE_BINDING=<<clusterrolebinding>>
oc get clusterrolebinding cluster-admin -o yaml | \
yq e 'del(.metadata.creationTimestamp)' - \
| yq e 'del(.metadata.resourceVersion)' - \
| yq e 'del(.metadata.selfLink)' - \
| yq e 'del(.metadata.managedFields)' - \
| yq e 'del(.metadata.uid)' - 
```

To export all the ClusterRoleBindings relevant to the filtered list of ClusterRoles in `clusterconfigs/cluster/cluster-roles` folder, run the following script. This will save the ClusterRoleBindings into `clusterconfigs/cluster/cluster-role-bindings` folder.

```
mkdir -p clusterconfigs/cluster/cluster-role-bindings
for role in $(ls clusterconfigs/cluster/cluster-roles | sed -e 's/\.yaml$//'); do \
  cmd=(oc get clusterrolebindings  -o jsonpath='{.items[?(@.roleRef.name == ROLE)].metadata.name}'); \
  cmd[4]=${cmd[4]//ROLE/\"$role\"}; \
  for i in $("${cmd[@]}"); do \
  echo "Exporting ClusterRoleBinding: " $i; \
  oc get clusterrolebinding $i -o yaml | \
  yq e 'del(.metadata.creationTimestamp)' - \
  | yq e 'del(.metadata.resourceVersion)' - \
  | yq e 'del(.metadata.selfLink)' - \
  | yq e 'del(.metadata.managedFields)' - \
  | yq e 'del(.metadata.uid)' -  - > clusterconfigs/cluster/cluster-role-bindings/$i.yaml; \
done; done
```

Verify the ClusterRoles and ClusterRoleBindings together again and remove those that are not relevant to target cluster.

  
# 6. Migrate Project Roles, Service Accounts and RoleBindings

In this section we will export project level Roles, ServiceAccounts and RoleBindings associated with Roles, ServiceAccounts and ClusterRoles.

### Get Project Roles for an OpenShift Project

If you are looking to export individual roles for a specific project, use this section.

List roles in a particular project

```
oc get roles -n OPENSHIFT_PROJECT
```

To get manifest for a specific role in a project
```
ROLE=<<rolename>>
OPENSHIFT_PROJECT=<<openshiftproject>>
oc get role $ROLE -n $OPENSHIFT_PROJECT -o yaml \
    | yq e 'del(.metadata.creationTimestamp)' - \
    | yq e 'del(.metadata.resourceVersion)' - \
    | yq e 'del(.metadata.selfLink)' - \
    | yq e 'del(.metadata.uid)' - \
    | yq e 'del(.metadata.managedFields)' - \
    | yq e 'del(.metadata.ownerReferences)' - \
    | yq e 'del(.status)' -
```

### Export all the Roles across all the selected Namespaces

Export roles across all the selected namespaces in the `clusterconfigs/namespaces` folder. The following script will export the roles into `clusterconfigs/namespaces/NAMESPACE` folder.

```
for ns in $(ls clusterconfigs/namespaces); do 
    echo "Exporting roles for namespace:" $ns; \
    for i in $(oc get roles -n $ns -o jsonpath='{.items[*].metadata.name}'); do
        oc get role $i -n $ns -o yaml \
        | yq e 'del(.metadata.creationTimestamp)' - \
        | yq e 'del(.metadata.resourceVersion)' - \
        | yq e 'del(.metadata.selfLink)' - \
        | yq e 'del(.metadata.uid)' - \
        | yq e 'del(.metadata.managedFields)' - \
        | yq e 'del(.metadata.ownerReferences)' - \
        | yq e 'del(.status)' -  > clusterconfigs/namespaces/$ns/$i-role.yaml
    done;
done
```

### Export Service Accounts for an OpenShift Project

If you are handling service accounts at the individual project level, use this.

To list service accounts in a particular project run:

```
oc get sa -n OPENSHIFT_PROJECT
```

To get SA manifest for a specific SA in a project. This will also remove references to secrets from this SA.

```
SA=<<serviceaccount>>
OPENSHIFT_PROJECT=<<openshiftproject>>
oc get sa $SA -n $OPENSHIFT_PROJECT -o yaml \
    | yq e 'del(.metadata.creationTimestamp)' - \
    | yq e 'del(.metadata.resourceVersion)' - \
    | yq e 'del(.metadata.selfLink)' - \
    | yq e 'del(.metadata.uid)' - \
    | yq e 'del(.metadata.managedFields)' - \
    | yq e 'del(.metadata.ownerReferences)' - \
    | yq e 'del(.status)' - \
    | yq e 'del(.secrets)' - \
    | yq e 'del(.imagePullSecrets)' - 

```

OpenShift creates some default service accounts for each openshift project viz., deployer, builder, default, and pipeline. Other than these there may be workload specific service accounts. To display workload specific service accounts in an OpenShift Project run :

```
SA_FILTERS="deployer\|builder\|default\|pipeline"
for i in $(oc get -n $OPENSHIFT_PROJECT sa -o jsonpath='{.items[*].metadata.name}'); do 
  if grep -v "$SA_FILTERS" <<< $i ; then 
     echo $i 
  fi 
done
```

### Export Service Accounts across all namespaces

The script below exports user created/workload specific service accounts across all namespaces and stores them in the service accounts folder in `clusterconfigs/namespaces/NAMESPACE` folder.

```
SA_FILTERS="deployer\|builder\|default\|pipeline"
for ns in $(ls clusterconfigs/namespaces); do 
    echo "Exporting service accounts for namespace:" $ns; \
    for i in $(oc get sa -n $ns -o jsonpath='{.items[*].metadata.name}'); do
        if grep -v "$SA_FILTERS" <<< $i ; then 
            oc get sa $i -n $ns -o yaml \
            | yq e 'del(.metadata.creationTimestamp)' - \
            | yq e 'del(.metadata.resourceVersion)' - \
            | yq e 'del(.metadata.selfLink)' - \
            | yq e 'del(.metadata.uid)' - \
            | yq e 'del(.metadata.managedFields)' - \
            | yq e 'del(.metadata.ownerReferences)' - \
            | yq e 'del(.secrets)' - \
            | yq e 'del(.imagePullSecrets)' - \
            | yq e 'del(.status)' -  > clusterconfigs/namespaces/$ns/$i-sa.yaml
        fi
    done;
done
```


### Export RoleBinding

If you are trying to export individual rolebindings for a specific OpenShift Project, look at this section.

To get manifest for a specific role binding for a particular openshift project, run

```
RB=<<rolebinding>>
OPENSHIFT_PROJECT=<<openshiftproject>>
oc get rolebinding $RB -n $OPENSHIFT_PROJECT -o yaml \
| yq e 'del(.metadata.creationTimestamp)' - \
| yq e 'del(.metadata.resourceVersion)' - \
| yq e 'del(.metadata.selfLink)' - \
| yq e 'del(.metadata.managedFields)' - \
| yq e 'del(.metadata.annotations)' - \
| yq e 'del(.metadata.ownerReferences)' - \
| yq e 'del(.metadata.labels)' - \
| yq e 'del(.metadata.uid)' - 
```

### Expert RoleBindings for all Roles and Service Accounts

Export the role bindings relevant to the roles and service accounts selected above. These will be stored in `clusterconfigs/namespaces/NAMESPACE` folder


```
for ns in $(ls clusterconfigs/namespaces); do 
    for role in $(ls clusterconfigs/namespaces/$ns/*-role.yaml 2> /dev/null | xargs -n 1 basename 2> /dev/null | sed -e 's/-role\.yaml$//'); do
        cmd=(oc get rolebindings -o jsonpath='{.items[?(@.roleRef.name == ROLE)].metadata.name}' -n NAMESPACE); \
        cmd[4]=${cmd[4]//ROLE/\"$role\"}; \
        cmd[6]=${cmd[6]//NAMESPACE/$ns}; \
        for i in $("${cmd[@]}"); do \
            echo "Exporting Rolebinding Namespace: " $ns "Role: " $role "RB: " $i 
            oc get rolebinding $i -n $ns -o yaml \
            | yq e 'del(.metadata.creationTimestamp)' - \
            | yq e 'del(.metadata.resourceVersion)' - \
            | yq e 'del(.metadata.selfLink)' - \
            | yq e 'del(.metadata.managedFields)' - \
            | yq e 'del(.metadata.annotations)' - \
            | yq e 'del(.metadata.ownerReferences)' - \
            | yq e 'del(.metadata.labels)' - \
            | yq e 'del(.metadata.uid)' - > clusterconfigs/namespaces/$ns/$i-rolebinding.yaml
        done;
    done;
done

for ns in $(ls clusterconfigs/namespaces); do 
    for sa in $(ls clusterconfigs/namespaces/$ns/*-sa.yaml 2> /dev/null | xargs -n 1 basename 2> /dev/null | sed -e 's/-sa\.yaml$//'); do
        for i in $(oc get rolebindings -n $ns -o yaml | yq e '.items[] | select(.subjects[].name == "'$sa'") | .metadata.name' -); do
            echo "Exporting Rolebinding Namespace: " $ns "SA: " $sa "RB: " $i
            oc get rolebinding $i -n $ns -o yaml \
            | yq e 'del(.metadata.creationTimestamp)' - \
            | yq e 'del(.metadata.resourceVersion)' - \
            | yq e 'del(.metadata.selfLink)' - \
            | yq e 'del(.metadata.managedFields)' - \
            | yq e 'del(.metadata.annotations)' - \
            | yq e 'del(.metadata.ownerReferences)' - \
            | yq e 'del(.metadata.labels)' - \
            | yq e 'del(.metadata.uid)' - > clusterconfigs/namespaces/$ns/$i-rolebinding.yaml
        done;
    done;
done
```

### RoleBindings associated with ClusterRoles

OpenShift creates certain rolebindings associated with cluster roles. These can be **optionally applied after reviewing them individually** if the cluster roles are being used in the target cluster. The following script will export all the rolebindings associated with cluster roles to `clusterconfigs/to-review/namespace-clusterrole-bindings/namespaces/NAMESPACE` folder. Once you review if you want to apply them, you can move to respective namespace folder. 

```
for ns in $(ls clusterconfigs/namespaces); do 
    for clusterrole in $(ls clusterconfigs/cluster/cluster-roles | sed -e 's/\.yaml$//'); do
        for i in $(oc get rolebindings -n $ns -o yaml | yq e '.items[] | select(.roleRef.name == "'$clusterrole'") | .metadata.name' -); do
            echo "Exporting Rolebinding Namespace: " $ns "CR: " $clusterrole "RB: " $i
            mkdir -p clusterconfigs/to-review/namespace-clusterrole-bindings/namespaces/$ns
            oc get rolebinding $i -n $ns -o yaml \
            | yq e 'del(.metadata.creationTimestamp)' - \
            | yq e 'del(.metadata.resourceVersion)' - \
            | yq e 'del(.metadata.selfLink)' - \
            | yq e 'del(.metadata.managedFields)' - \
            | yq e 'del(.metadata.annotations)' - \
            | yq e 'del(.metadata.ownerReferences)' - \
            | yq e 'del(.metadata.labels)' - \
            | yq e 'del(.metadata.uid)' - > clusterconfigs/to-review/namespace-clusterrole-bindings/namespaces/$ns/$i-rolebinding.yaml
        done;
    done;
done
```

# 7. Export Application Related Manifests


### Export Deployment Configs

```
for ns in $(ls clusterconfigs/namespaces); do
    echo "Exporting manifests for namespace: " $ns;
    mkdir -p ocp-manifests/namespaces/$ns;
    for dc in $(oc get dc -n $ns -o jsonpath='{.items[*].metadata.name}' ); do
        echo "Exporting DeploymentConfigs: " $dc;
        oc get dc $dc -n $ns -o yaml \
            | yq e 'del(.metadata.creationTimestamp)' - \
            | yq e 'del(.metadata.annotations.*)' - \
            | yq e 'del(.metadata.labels.template*)' - \
            | yq e 'del(.metadata.labels.xpaas)' - \
            | yq e 'del(.metadata.resourceVersion)' - \
            | yq e 'del(.metadata.selfLink)' - \
            | yq e 'del(.metadata.uid)' - \
            | yq e 'del(.metadata.generation)' - \
            | yq e 'del(.metadata.managedFields)' - \
            | yq e 'del(.status)' -  \
            > ocp-manifests/namespaces/$ns/$dc-dc.yaml;
    done;
    
done
```

### Export Deployments
```
for ns in $(ls clusterconfigs/namespaces); do
    echo "Exporting manifests for namespace: " $ns;
    mkdir -p ocp-manifests/namespaces/$ns;
    for deployment in $(oc get deployments -n $ns -o jsonpath='{.items[*].metadata.name}' ); do
        echo "Exporting Deployments: " $deployment;
        oc get deployment $deployment -n $ns -o yaml \
            | yq e 'del(.metadata.creationTimestamp)' - \
            | yq e 'del(.metadata.annotations.*)' - \
            | yq e 'del(.metadata.labels.template*)' - \
            | yq e 'del(.metadata.labels.xpaas)' - \
            | yq e 'del(.metadata.resourceVersion)' - \
            | yq e 'del(.metadata.selfLink)' - \
            | yq e 'del(.metadata.uid)' - \
            | yq e 'del(.metadata.generation)' - \
            | yq e 'del(.metadata.managedFields)' - \
            | yq e 'del(.status)' - \
            > ocp-manifests/namespaces/$ns/$deployment-deployment.yaml
    done;
done
```

### Export Secrets

**CAUTION:** You may not want to export secrets to a git repository

```
SECRET_FILTERS="^builder-\|^deployer-\|^default-\|^pipeline-"
oc get secrets -n demo -o jsonpath='{.items[*].metadata.name}' | grep -v $SECRET_FILTERS
for ns in $(ls clusterconfigs/namespaces); do
    echo "Exporting manifests for namespace: " $ns;
    mkdir -p ocp-manifests/namespaces/$ns;
    for secret in $(oc get secrets -n $ns -o jsonpath='{.items[*].metadata.name}' ); do
    if grep -v "$SECRET_FILTERS" <<< $secret ; then
        echo "Exporting Secret: " $secret;
            oc get secret $secret -n $ns -o yaml \
                | yq e 'del(.metadata.creationTimestamp)' - \
                | yq e 'del(.metadata.annotations.*)' - \
                | yq e 'del(.metadata.resourceVersion)' - \
                | yq e 'del(.metadata.selfLink)' - \
                | yq e 'del(.metadata.uid)' - \
                | yq e 'del(.metadata.generateName)' - \
                | yq e 'del(.metadata.managedFields)' - \
                | yq e 'del(.status)' - \
                > ocp-manifests/namespaces/$ns/$secret-secret.yaml
    fi
    done;
done
```

### Export ImageStreams

```
for ns in $(ls clusterconfigs/namespaces); do
    echo "Exporting manifests for namespace: " $ns;
    mkdir -p ocp-manifests/namespaces/$ns;
    for is in $(oc get is -n $ns -o jsonpath='{.items[*].metadata.name}' ); do
        echo "Exporting ImageStreams: " $is;
        oc get is $is -n $ns -o yaml \
            | yq e 'del(.metadata.creationTimestamp)' - \
            | yq e 'del(.metadata.annotations.*)' - \
            | yq e 'del(.metadata.resourceVersion)' - \
            | yq e 'del(.metadata.selfLink)' - \
            | yq e 'del(.metadata.generation)' - \
            | yq e 'del(.metadata.managedFields)' - \
            | yq e 'del(.metadata.uid)' - \
            > ocp-manifests/namespaces/$ns/$is-is.yaml
    done;
done
```

### Export Services

```
for ns in $(ls clusterconfigs/namespaces); do
    echo "Exporting manifests for namespace: " $ns;
    mkdir -p ocp-manifests/namespaces/$ns;
    for service in $(oc get service -n $ns -o jsonpath='{.items[*].metadata.name}' ); do
        echo "Exporting service: " $service;
        oc get svc $service -n $ns -o yaml \
            | yq e 'del(.metadata.creationTimestamp)' - \
            | yq e 'del(.metadata.annotations.*)' - \
            | yq e 'del(.metadata.labels.template*)' - \
            | yq e 'del(.metadata.labels.xpaas)' - \
            | yq e 'del(.metadata.resourceVersion)' - \
            | yq e 'del(.metadata.selfLink)' - \
            | yq e 'del(.metadata.uid)' - \
            | yq e 'del(.metadata.managedFields)' - \
            | yq e 'del(.status)' -  \
            | yq e 'del(.spec.clusterIP)' -  \
            | yq e 'del(.spec.clusterIPs)' -  \
            > ocp-manifests/namespaces/$ns/$service-service.yaml

    done;
done

```

### Export Routes

```
for ns in $(ls clusterconfigs/namespaces); do
    echo "Exporting manifests for namespace: " $ns;
    mkdir -p ocp-manifests/namespaces/$ns;
    for route in $(oc get route -n $ns -o jsonpath='{.items[*].metadata.name}' ); do
        echo "Exporting Route: " $route;
        oc get route $route -n $ns -o yaml \
            | yq e 'del(.metadata.creationTimestamp)' - \
            | yq e 'del(.metadata.annotations.*)' - \
            | yq e 'del(.metadata.labels.template*)' - \
            | yq e 'del(.metadata.labels.xpaas)' - \
            | yq e 'del(.metadata.resourceVersion)' - \
            | yq e 'del(.metadata.selfLink)' - \
            | yq e 'del(.metadata.managedFields)' - \
            | yq e 'del(.metadata.uid)' - \
            | yq e 'del(.status)' - \
            > ocp-manifests/namespaces/$ns/$route-route.yaml

    done;
done

```

### Export ConfigMaps

```
for ns in $(ls clusterconfigs/namespaces); do
    echo "Exporting manifests for namespace: " $ns;
    mkdir -p ocp-manifests/namespaces/$ns;
    for cm in $(oc get cm -n $ns -o jsonpath='{.items[*].metadata.name}' ); do
        echo "Exporting ConfigMap: " $cm;
        oc get cm $cm -n $ns -o yaml \
            | yq e 'del(.metadata.creationTimestamp)' - \
            | yq e 'del(.metadata.annotations.*)' - \
            | yq e 'del(.metadata.resourceVersion)' - \
            | yq e 'del(.metadata.selfLink)' - \
            | yq e 'del(.metadata.managedFields)' - \
            | yq e 'del(.metadata.uid)' - \
            | yq e 'del(.status)' - \
            > ocp-manifests/namespaces/$ns/$cm-cm.yaml

    done;
done
 
```

### Export Persistent Volume Claims

```

for ns in $(ls clusterconfigs/namespaces); do
    echo "Exporting manifests for namespace: " $ns;
    mkdir -p ocp-manifests/namespaces/$ns;
    for pvc in $(oc get pvc -n $ns -o jsonpath='{.items[*].metadata.name}' ); do
        echo "Exporting PVC: " $pvc;
        oc get pvc $pvc -n $ns -o yaml \
            | yq e 'del(.metadata.creationTimestamp)' - \
            | yq e 'del(.metadata.annotations.*)' - \
            | yq e 'del(.metadata.resourceVersion)' - \
            | yq e 'del(.metadata.selfLink)' - \
            | yq e 'del(.metadata.managedFields)' - \
            | yq e 'del(.metadata.uid)' - \
            | yq e 'del(.status)' - \
            > ocp-manifests/namespaces/$ns/$pvc-pvc.yaml

    done;
done

```

# 8. Image Migration from OpenShift Internal Registry to ECR (Elastic Container Registry)

If you are using OpenShift Internal Registry to store the application container images, you will need to export these images to a different registry to deploy them to the target cluster. If you are already using an external registry, you don't need to follow the steps explained in this section.

While you can use any container registry as the target container registry, this example uses GCR as the container registry. 


The steps below are tested on a Linux box. These do not work on Google Cloud Shell. 

* Install Docker on the host.


### Connecting to OpenShift Internal Registry 

* Get OpenShift Registry URL. 

    If you are using **OpenShift 4.x** expose the registry as `default_route` following the instruction [here](https://docs.openshift.com/container-platform/4.7/registry/securing-exposing-registry.html). 

    ```
    REGISTRY_URL=$(oc get route default-route -n openshift-image-registry -o jsonpath='{.spec.host}')
    ```

    If you are using **OpenShift 3.x**, you will have to expose it yourself, as below. Confirm that the registry service is running in `default` OpenShift project and the service name is `image-registry` before running these commands , if not you may have to change these respective values.

    ```
    oc project default
    oc create route reencrypt --service=image-registry
    REGISTRY_URL=$(oc get route image-registry -n default -o jsonpath='{.spec.host}')
    ```


* Get Root CA certificate from the created route. The command below saves the cert in a file with name `registry_ca.crt`. 
```
openssl s_client -showcerts -connect ${REGISTRY_URL}:443 < /dev/null |  awk '/BEGIN/ {c=1; print >"registry_ca.crt"; next} /END/ {print >"registry_ca.crt"; exit}; c{print >"registry_ca.crt"}'

```

* Copy this file to `/etc/docker/certs.d` folder.

```
sudo mkdir -p /etc/docker/certs.d/$REGISTRY_URL
sudo cp registry_ca.crt /etc/docker/certs.d/$REGISTRY_URL/ca.crt
```

* Restart Docker

```
sudo systemctl restart docker
```

* Login to the OpenShift internal registry

```
sudo docker login -u `oc whoami` -p `oc whoami -t` $REGISTRY_URL
```
You should see a message `Login Succeeded`

### Connecting to Target GCR 

```
gcloud auth print-access-token | sudo docker login -u oauth2accesstoken --password-stdin https://gcr.io
```
You should see a message `Login Succeeded`


### Migrate Images for an Application

For the application on the namespace you are trying to migrate, you can migrate the container images from source OpenShift Internal Registry to the target GCR repository as follows:

* Find the public URL of the source image. The imagestreams are exported to `ocp-manifests/namespaces` folder in the previous step and they all end with `*-is.yaml`

```
$ ls ocp-manifests/namespaces/$NAMESPACE/*-is.yaml

ocp-manifests/namespaces/development/myapp-is.yaml
```

* Get the name of the source container image from this image stream 

```
SOURCE_IMAGE=$(cat ocp-manifests/namespaces/development/myapp-is.yaml | yq e '.status.publicDockerImageRepository' - ):latest
```
* Pull the source image using `docker pull`

```
sudo docker pull $SOURCE_IMAGE
```
* Tag the image to the target repository. Substitute the value of the TARGET_REPO with your value before running this.

```
TARGET_REPO="gcr.io\/<<your-project>>"

export TARGET_IMAGE=$(echo $SOURCE_IMAGE | sed -e 's/'$REGISTRY_URL'/'$TARGET_REPO'/')
sudo docker tag $SOURCE_IMAGE $TARGET_IMAGE
```
* Push the image to target registry

```
sudo docker push $TARGET_IMAGE
```

If there are multiple imagestreams in the namespace, repeat the above steps for all image streams to migrate all the images.  


### Migrate all Images from the Cluster

If you want to migrate all the container images for the selected namespaces from source OpenShift Internal Registry to target GCR registry instead of doing it on a per namespace basis, run this script. This script may take several hours to run depending on the number of images to be migrated, their size and your connection speed to the source and target registries.

**NOTE:** Set the appropriate value for TARGET_REPO before running the script.

```
TARGET_REPO="gcr.io\/<<your-project>>"
```

```
for is in $(find ocp-manifests/ -name *-is.yaml); do
    SOURCE_IMAGE=$(cat $is | yq e '.status.publicDockerImageRepository' -):latest;
    echo "Pulling Image:" $SOURCE_IMAGE;
    sudo docker pull $SOURCE_IMAGE;
    TARGET_IMAGE=$(echo $SOURCE_IMAGE | sed -e 's/'$REGISTRY_URL'/'$TARGET_REPO'/');
    sudo docker tag $SOURCE_IMAGE $TARGET_IMAGE;
    echo "Pushing Image:" $TARGET_IMAGE;
    sudo docker push $TARGET_IMAGE
    sudo docker rmi $SOURCE_IMAGE;
    sudo docker rmi $TARGET_IMAGE;
done
```


# Migrate Applications without Persistent Storage (Stateless Applications)

This section addresses migrating applications by converting application manifests from the source format that were exported to `ocp-manifests/namespaces` folder to the target kubernetes manifests that will be saved into `clusterconfigs/namespaces`.

While it is possible to script out the entire workload migration, we recommend handling this migration one namespace at a time. This allows you to verify the workloads in each namespace are running before moving on to the next one. Also you can selectively migrate the workloads by prioritizing them, resolving any dependencies and by making sure that unnecessary workloads are not migrated.

We will be using [Shifter tool](https://github.com/garybowers/shifter) to convert the manifests from [OpenShift format to Kubernetes format](https://github.com/garybowers/shifter#openshift-to-kubernetes-converter). Download the latest release of Shifter [from here](https://github.com/garybowers/shifter/releases) onto your linux box.

```
wget https://github.com/garybowers/shifter/releases/download/v0.11/shifter-linux-amd64
mv shifter-linux-amd64 shifter
chmod +x shifter
```
Also set the PATH to access shifter or move it to a place where the PATH is already set.

### Converting Manifests

* Select a namespace you want to migrate. You can select one from the list of namespaces for which the manifests were exported previously `ls ocp-manifests/namespaces/`

```
NAMESPACE=<<selected namespace>>
```

* Set output location for the resultant target manifests. This can be set to `clusterconfigs/namespaces/$NAMESPACE` folder if you want to use ACM to deploy these manifests.

```
OUTPUT_LOCATION=clusterconfigs/namespaces/$NAMESPACE
```

* Find the list of manifests exported from the OpenShift cluster 

```
INPUT_LOCATION=ocp-manifests/namespaces/$NAMESPACE
ls $INPUT_LOCATION
```

* Convert each OpenShift DeploymentConfig listed to Kubernetes Deployments using Shifter. The script below creates a deployment for each manifest it the output folder with the same name as the openshift deployment configuration but ending in `deployment.yaml`.

```
for dc in $(ls $INPUT_LOCATION/*-dc.yaml); do
    echo "converting DC: " $dc
    OUTPUT_FILE=$(echo $dc | sed -e 's/ocp-manifests/clusterconfigs/' -e 's/-dc/-deployment/' -);
    shifter convert -i yaml -t yaml -f $dc -o $OUTPUT_FILE;
done
```

* Copy any Deployments, Services and ConfigMaps. There is no need to convert these manifests.

```
cp $INPUT_LOCATION/*-deployment.yaml $OUTPUT_LOCATION
cp $INPUT_LOCATION/*-service.yaml $OUTPUT_LOCATION
cp $INPUT_LOCATION/*-cm.yaml $OUTPUT_LOCATION
```

* If you have exported secrets earlier, copy any secrets to the target location. Again no conversion needed in this case
```
cp $INPUT_LOCATION/*-secret.yaml $OUTPUT_LOCATION
```

* Convert OpenShift Routes to Kubernetes Ingress Objects

```
for route in $(ls $INPUT_LOCATION/*-route.yaml); do
    echo "Converting Route: " $route
    OUTPUT_FILE=$(echo $route | sed -e 's/ocp-manifests/clusterconfigs/' -e 's/-route/-ingress/' -);
    shifter convert -i yaml -t yaml -f $route -o $OUTPUT_FILE;
done
```

* If you have Persistent volumes to migrate, follow the procedure [here](WIP)

### Create Image Pull Secret 

The target cluster may require credentials to pull images from a container registry, if the registry is protected. In this section, we will see how to set up a secret to the container registry. We will use GCR as an example.

If you are using GCR as the registry, in the previous section you have seen how to [transfer images from OpenShift Internal Registry to GCR](./11.TransferApplicationImages.md). 

To provide access to your application deployment to pull images from this registry, create a service account that can pull images from the private registry, download the service account key and create a secret [as explained in this article](https://cloud.google.com/anthos/clusters/docs/aws/how-to/private-registry)

Once you download the key, create a secret in the target namespace with the key as shown below. Change the name of the secret, if you want to.

```
IMAGEPULLSECRET=gcr-secret
SERVICE_ACCOUNT_EMAIL=registry-sa@ocptogkedemoproject.iam.gserviceaccount.com 

kubectl create secret docker-registry $IMAGEPULLSECRET         --docker-server=gcr.io         --docker-username=_json_key         --docker-email=$SERVICE_ACCOUNT_EMAIL    --docker-password="$(cat key.json)" -n $NAMESPACE
```

### Update the deployments to point to Target Registry

* **If the images have been migrated** to [a different registry as part of migration](./11.TransferApplicationImages.md) from OpenShift Internal Registry, change the container image in the deployment to point to the target repository where the image is moved.

    A deployment in OpenShift would be referencing the image from internal registry using its [internal DNS name](https://kubernetes.io/docs/concepts/services-networking/service/#dns). In an OpenShift 4.x cluster this is always set to `image-registry.openshift-image-registry.svc:5000` as the registry service is named `image-registry` and runs in namespace `openshift-image-registry` by the openshift image registry operator. You can verify this by looking at the deployment. Set appropriate values for OpenShift `INTERNAL_REGISTRY_URL` and the `TARGET_REPO`.

```
INTERNAL_REGISTRY_URL=image-registry.openshift-image-registry.svc:5000
TARGET_REPO="gcr.io\/<<your-project>>"
```

**NOTE**:
* OpenShift uses specific image shaid in the deployment. Sometimes the target cluster doesnt recognize the specific image id and you may see error like this `Failed to fetch "sha256:8537f5b2211c69e07b9a855ba661ee5b218486aab4b41ed4f0d199e22ce34e30" from request`. In order to prevent this issue we will replace shaid with the `latest` tag. 

Run the following code snippet to replace the images in the deployment to point to the target repository and to the `latest` image as some images may have shaid that may not work. 

```
for deployment in $(ls $OUTPUT_LOCATION/*-deployment.yaml); do
    CHANGED_IMAGE=$(cat $deployment | yq e '.spec.template.spec.containers[].image' - | sed -e 's@'$INTERNAL_REGISTRY_URL'@'$TARGET_REPO'@' -e 's/@sha256:.*$/:latest/') ;
    cat $deployment \
    | yq e '.spec.template.spec.containers[].image |= "'$CHANGED_IMAGE'"' - \
    > $deployment.new;
    mv $deployment $deployment.bak
    mv $deployment.new $deployment
done

```

**NOTE** 
* Sometimes the image names are inconsistent between the imagestream public repository name and the one used in the deployment configurations. Verify the resultant deployment reference to the image and the image actually migrated to the target repository to make sure they are the same. If required, you may have to change the image name



### Apply Security Constraints to the Deployments


* Generate secure deployments, depending on the policies that apply to your namespace. Refer the [ACM Security Polices that were set up earlier](./8.SetupRestrictedConstraints.md). If your application runs with [restricted security constraints](./policies/restricted/restricted_constraints.yaml), we have to update deployments with security settings that meet those constraints. In the example below, we will apply specific USERID, FSGROUPID, drop capabilities.
Also, if we are pulling from a registry that requires credentials, that were configured in the previous steps, we will configure the secret as the image pull secret. Set the values of USERID, FSGROUPID and IMAGEPULLSECRET to appropriate values before running the script below.

```
USERID=1001
FSGROUPID=1101

for deployment in $(ls $OUTPUT_LOCATION/*-deployment.yaml); do
    echo "Generating Restricted Deployment for : " $deployment;
    cat $deployment \
    | yq e 'del(.spec.template.spec.securityContext)'  - \
    | yq e '.spec.template.spec.securityContext.runAsUser |= '$USERID'' - \
    | yq e '.spec.template.spec.securityContext.fsGroup |= '$FSGROUPID'' - \
    | yq e '.spec.template.spec.containers[].securityContext.capabilities.drop |= ["KILL","MKNOD","SYS_CHROOT"]' - \
    | yq e '.spec.template.spec.initContainers[].securityContext.capabilities.drop |= ["KILL","MKNOD","SYS_CHROOT"]' - \
    | yq e '.spec.template.spec.imagePullSecrets[0].name |= "'$IMAGEPULLSECRET'"' - \
    > $deployment.new;
    mv $deployment $deployment.bak
    mv $deployment.new $deployment
done;
```

Verify the deployments generated to make sure they are good with the changes.


### Applying Application Manifests to the Target Cluster

* Connect to the Target GKE Cluster, if you are not already connected

```
kubectx <<target cluster>>
```

* Apply ConfigMaps to the target cluster

```
for cm in $(ls $OUTPUT_LOCATION/*-cm.yaml); do
    kubectl -n $NAMESPACE apply -f $cm
done
```

* Apply secrets, if any, to the target cluster. **NOTE** Generally, you may create these secrets directly instead of importing from the source cluster.

```
for secret in $(ls $OUTPUT_LOCATION/*-secret.yaml); do
    kubectl -n $NAMESPACE apply -f $secret
done
```

* Apply the deployments to the target cluster
```
for deployment in $(ls $OUTPUT_LOCATION/*-deployment.yaml); do
    kubectl -n $NAMESPACE apply -f $deployment
done
```
Verify that the pods are running. You can look at `kubectl get events -n $NAMESPACE --sort-by=lastTimestamp` to debug in case of any issues.


You have two choices while creating services. For the services that are externally reachable you can either use Kubernetes Ingress or you can use LoadBalancer type service.

### Applying Kubernetes Ingress

* Apply the services to the target cluster
```
for svc in $(ls $OUTPUT_LOCATION/*-service.yaml); do
    kubectl -n $NAMESPACE apply -f $svc
done
```

* Apply the kubernetes ingresses to the target cluster
```
for ingress in $(ls $OUTPUT_LOCATION/*-ingress.yaml); do
    kubectl -n $NAMESPACE apply -f $ingress
done
```

### Applying Loadbalancer type service

In this case identify the services that need to be reachable from outside the cluster. Only those externally accessible services should be changes to `LoadBalancer` type.

* For each service that needs to be exposed run after replacing the service filename with appropriate value.

```
SERVICE=$OUTPUT_LOCATION/<<servicefilename>>

cat $SERVICE \
| yq e '.spec.type |= "LoadBalancer"' - | kubectl -n $NAMESPACE apply -f -
```

Once you create all services verify that these LoadBalancer type services are assigned ExternalIP by running `kubectl get svc -n $NAMESPACE`. 

**NOTE** 
* It may take a few minutes for the External_IP to be assigned.
* K8S Services configured in OpenShift usually don't use port 80 , but 8080, by default for http traffic. So if you are exposing services as LoadBalancer type without changing ports, verify the port number and access the service on the port exposed.

Access the application by using any of the above to ingress mechanisms using External IP.


# Migrate Applications with Persistent Storage using Velero

This section further expands our migration capability to include both the persistent data and workloads. From previous sections, we learned Openshift has several propietary objects (e.g. DeploymentConfig, Route), which are different from Kubernetes. Luckily, when it comes to Persistent Data, Openshift sticks with Kubernetes standard. Therefore, we could achieve the migration with the help of Opensource data migration tools. 

In this demo, we will use [Velero](https://velero.io/) as an example. However, there are other 3rd-party tools (e.g. Kasten from Veeam) could also be used. Readers could select one which fits your work. Like our earlier work, we will [Shifter](https://github.com/garybowers/shifter) for workloads migration. Please have both Velero CLI and Shifter CLI installed onto your workstation. We'll conduct our demo in Mac OS to show our shifter tool could be used cross-platform. In fact, shifter supports [MAC/Linux/Windows starting from 0.12](https://github.com/garybowers/shifter/releases). For linux tutorial, please refer to [11.MigrateApplications](https://github.com/VeerMuchandi/MigratingFromOpenShiftToGKE/blob/main/11.MigrateApplications.md)

```
# Download shifter CLI
wget https://github.com/garybowers/shifter/releases/download/0.12/shifter_darwin_amd64
mv shifter-darwin-amd64 shifter
chmod +x shifter

# Download velero CLI
wget https://github.com/vmware-tanzu/velero/releases/download/v1.6.0/velero-v1.6.0-darwin-amd64.tar.gz
tar zxvf velero-v1.6.0-darwin-amd64.tar.gz
mv velero-v1.6.0-darwin-amd64/velero ./velero
chmod +x velero
```
Also set the PATH to access shifter or move it to a place where the PATH is already set.

We recommend conducting migration processes per namespace instead of the entire cluster. Also, a switch downtime is expected for the migration process, which normally can be minutes to hours, depending on the workload types. The following diagram provides a quick sketch for our
![lab-setup](images/lab-setup.png)
## Create Bucket in Google Cloud Storage
In order to utilize velero for persistent data backup/restore, the following script could create a bucket in Google Cloud Storage (GCS) and the service account to read/write to this bucket. This script also downloads the service account key as credential-velero file for the next step. 
```console
# Please replace {PROJECT_ID} and {BUCKET_NAME} with your input.
export PROJECT_ID={PROJECT_ID}
export BUCKET={BUCKET_NAME}

gsutil mb gs://$BUCKET/
gcloud iam service-accounts create velero \
    --display-name "Velero service account"

SERVICE_ACCOUNT_EMAIL=$(gcloud iam service-accounts list \
  --filter="displayName:Velero service account" \
  --format 'value(email)')

ROLE_PERMISSIONS=(
    compute.disks.get
    compute.disks.create
    compute.disks.createSnapshot
    compute.snapshots.get
    compute.snapshots.create
    compute.snapshots.useReadOnly
    compute.snapshots.delete
    compute.zones.get
)

gcloud iam roles create velero.server \
    --project $PROJECT_ID \
    --title "Velero Server" \
    --permissions "$(IFS=","; echo "${ROLE_PERMISSIONS[*]}")"

gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member serviceAccount:$SERVICE_ACCOUNT_EMAIL \
    --role projects/$PROJECT_ID/roles/velero.server

gsutil iam ch serviceAccount:$SERVICE_ACCOUNT_EMAIL:objectAdmin gs://${BUCKET}
gcloud iam service-accounts keys create credentials-velero \
    --iam-account $SERVICE_ACCOUNT_EMAIL
```
### Install Velero
Next, we could use the following script to install velero in both Openshift and Kubernetes (GKE) clusters. This script has to be executed with [kubectx](https://github.com/ahmetb/kubectx/releases) and [oc](https://docs.openshift.com/container-platform/3.6/cli_reference/get_started_cli.html#installing-the-cli) commands. Please provide the kubeconfig contexts and bucket name as the input of the script.
```console
export context_src={OPENSHIFT_CONTEXT}
export context_dst={GKE_CONTEXT}
export BUCKET={BUCKET_NAME}

kubectx ${context_src}
velero install \
    --provider gcp \
    --plugins velero/velero-plugin-for-gcp:v1.1.0 \
    --bucket $BUCKET \
    --use-restic \
    --secret-file credentials-velero

./oc -n velero patch ds/restic --type json -p '[{"op":"add","path":"/spec/template/spec/containers/0/securityContext","value": { "privileged": true}}]'
./oc -n velero patch ds/restic --type json -p '[{"op":"replace","path":"/spec/template/spec/volumes/0/hostPath","value": { "path": "/var/lib/kubelet/pods"}}]'

kubectx ${context_dst}
velero install \
    --provider gcp \
    --plugins velero/velero-plugin-for-gcp:v1.1.0 \
    --bucket $BUCKET \
    --use-restic \
    --secret-file credentials-velero
```
### Annotate workloads for Velero backup
Velero uses annotation to notify Restic the volumes to backup. Since the persistent volumes were attached to each pod, it became complicated when  Velero, after version 1.5, supports both [opt-in](https://velero.io/docs/v1.5/restic/#using-opt-in-pod-volume-backup) and [opt-out](https://velero.io/docs/v1.5/restic/#using-the-opt-out-approach) mechanims for persistent volume backup. Opt-in mode does not backup any persistent volume by default but only to the pods which have the annotation. Opt-out mode would backup all persistent volumes (except for secrets/configmaps and hostpath volumes) by default. Pods requires to be annotated to have its volume being excluded.

In this demo, we choose opt-out since we intended to backup all persistent volumes in the desired namespace. By adding "--default-volumes-to-restic" in the backup command could easily activate the opt-out mode. Compared with the opt-in mode which requires additional annotations, opt-out mode provides an easier way for existing openshift data migration.

### Converting Manifests
We took a similar approach as shown in [11.MigrateApplications#converting-manifests](https://github.com/VeerMuchandi/MigratingFromOpenShiftToGKE/blob/main/11.MigrateApplications.md#converting-manifests) to capture openshift objects into YAML format and stored in ocp-manifests/ folder. Then, shifter is applied to convert those openshift objects into standard kubernetes object yaml files and stored in kubernetes-manifests/ folder (shown in the last step).

```
export ns=${WORKLOAD_NAMESPACE}

# Capture workload
for dc in $(./oc get dc -n $ns -o jsonpath='{.items[*].metadata.name}' ); do
  echo "Exporting DeploymentConfigs: " $dc;
  ./oc get dc $dc -n $ns -o yaml \
    | yq e 'del(.metadata.creationTimestamp)' - \
    | yq e 'del(.metadata.annotations.*)' - \
    | yq e 'del(.metadata.labels.template*)' - \
    | yq e 'del(.metadata.labels.xpaas)' - \
    | yq e 'del(.metadata.resourceVersion)' - \
    | yq e 'del(.metadata.selfLink)' - \
    | yq e 'del(.metadata.uid)' - \
    | yq e 'del(.metadata.generation)' - \
    | yq e 'del(.metadata.managedFields)' - \
    | yq e 'del(.status)' -  \
    > ocp-manifests/namespaces/$ns/$dc-dc.yaml;
done

for route in $(./oc get route -n $ns -o jsonpath='{.items[*].metadata.name}' ); do
  echo "Exporting Route: " $route;
  ./oc get route $route -n $ns -o yaml \
    | yq e 'del(.metadata.creationTimestamp)' - \
    | yq e 'del(.metadata.annotations.*)' - \
    | yq e 'del(.metadata.labels.template*)' - \
    | yq e 'del(.metadata.labels.xpaas)' - \
    | yq e 'del(.metadata.resourceVersion)' - \
    | yq e 'del(.metadata.selfLink)' - \
    | yq e 'del(.metadata.managedFields)' - \
    | yq e 'del(.metadata.uid)' - \
    | yq e 'del(.status)' - \
    > ocp-manifests/namespaces/$ns/$route-route.yaml
done

#imagestreams
for is in $(./oc get is -n $ns -o jsonpath='{.items[*].metadata.name}' ); do
    echo "Exporting ImageStreams: " $is;
    ./oc get is $is -n $ns -o yaml \
        | yq e 'del(.metadata.creationTimestamp)' - \
        | yq e 'del(.metadata.annotations.*)' - \
        | yq e 'del(.metadata.resourceVersion)' - \
        | yq e 'del(.metadata.selfLink)' - \
        | yq e 'del(.metadata.generation)' - \
        | yq e 'del(.metadata.uid)' - \
        > ocp-manifests/namespaces/$ns/$is-is.yaml
done;

#services
for service in $(./oc get service -n $ns -o jsonpath='{.items[*].metadata.name}' ); do
    echo "Exporting service: " $service;
    ./oc get svc $service -n $ns -o yaml \
        | yq e 'del(.metadata.creationTimestamp)' - \
        | yq e 'del(.metadata.annotations.*)' - \
        | yq e 'del(.metadata.labels.template*)' - \
        | yq e 'del(.metadata.labels.xpaas)' - \
        | yq e 'del(.metadata.resourceVersion)' - \
        | yq e 'del(.metadata.selfLink)' - \
        | yq e 'del(.metadata.uid)' - \
        | yq e 'del(.metadata.managedFields)' - \
        | yq e 'del(.status)' -  \
        | yq e 'del(.spec.clusterIP)' -  \
        | yq e 'del(.spec.clusterIPs)' -  \
        > ocp-manifests/namespaces/$ns/$service-service.yaml
done;
./shifter convert -f ./ocp-manifests/namespaces/$ns -t yaml -o ./kubernetes-manifests/namespaces/$ns
```
### Select workloads for Velero Backup
Velero, by default, does not only backup persistent volume but also standard kubernetes objects (e.g. pods, services, config maps). Those components are included in deployment objects, converted by shifter in previous step. We therefore will adapt labelling techniques, provided by Velero. By properly label the PersistentVolume and PersistentVolumeClaim, we could successfully backup data without other kubernetes objects. The following script provides an example. 

```
export pvc_name={DESIRED_PVC_NAME}
# label format is 'key=value'
export mylabel={DESIRED_LABEL}

./oc label pvc ${pvc_name} ${mylabel}
my_pv=$(./oc get pv | grep ${pvc_name} | awk '{print $1}')
./oc label pv ${my_pv} ${mylabel}

velero backup create select-backup --selector ${mylabel} --default-volumes-to-restic
```
### Activate Migrated Workloads
Finally, we will activate the entire workloads. Let's first switch to the kubernetes cluster and use "velero backup get" to ensure the backup is successful. Note the the status of the backup success could be delay in target cluster compared with the source cluster. Once the backup is completed, by running the following two commands, we could have the entire workloads up-and-running in target cluster.
```
velero restore create ${restore-name} --from-backup ${backup-name}
```
Once the pv and pvc are ready, we could quickly run
```
export ns={WORKLOAD_NAMESPACE}
kubectl apply -f kubernetes-manifests/namespace/$ns/
```

To better demonstrate the steps above, an openshift workload has been prepared in [samples folder](https://github.com/GoogleCloudPlatform/migratingfromopenshifttoanthos/tree/main/samples). To use this sample, please modify envs.sh according to your environment. Then, run the scripts in the numerical order. 
  
