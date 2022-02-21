#!/bin/bash

# generate_dependency_licenses.sh
# Generate dependency package list in json
# How to use
# ./generate_dependency_licenses.sh [LICSENSE_TYPE1,LICENSE_TYPE_2,...]
# Example
# ./generate_dependency_licenses.sh BSD-3-Clause,BSD-2-Clause
#
# Please install go-license before running this script
# go get github.com/google/go-licenses

# Constants
PACKAGE="github.com/0xPolygon/polygon-edge"

# Arguments
licenses=( `echo $1 | tr -s ',' ' '`)

# Get the version of given package from go.mod or go.sum
get_package_version() {
  package=$1

  # Get version from go.mod first
  gomod_res=`cat go.mod | grep $package`
  if [ ${#gomod_res} -gt 0 ]; then
    res=(`echo $gomod_res | tr -s ',' ' '`)
    echo "${res[1]}"

    return 0
  fi

  # Get version from go.sum if not found
  # There may be multiple versions in go.sum and take the last
  tmp_ifs=$IFS
  IFS=$'\n'
  gosum_res=(`cat go.sum | grep $package`)
  IFS=$tmp_ifs

  if [ ${#gosum_res} -gt 0 ]; then
    last=${gosum_res[${#gosum_res[@]} - 1]}
    arr=(`echo $last | tr -s ',' ' '`)
    # Remove suffix from version
    version=`echo "${arr[1]}" | sed -e 's/\/go.mod//g' | sed -e 's/\+incompatible//g'`
    echo "$version"

    return 0
  fi
}

# Print package information in JSON
print_dependency_json() {
  name=$1
  license=$2
  path=$3
  version=$4

  if [ ${#version} -ne 0 ]
  then
    # with version
    printf '
  {
    "name": "%s",
    "version": "%s",
    "type": "%s",
    "path": "%s"
  }' "${name}" "${version}" "${license}" "${path}"
  else
    # without version
    printf '
  {
    "name": "%s",
    "type": "%s",
    "path": "%s"
  }' "${name}" "${license}" "${path}"
  fi
}

echo -n '['

count=0
# Run go-licenses and process each line
go-licenses csv $PACKAGE | while read line; do
  # Parse line
  res=(`echo $line | tr -s ',' ' '`)
  package=${res[0]}
  fullpath=${res[1]}
  license=${res[2]}

  # Filter by license
  if [ "$license" = "Unknown" ]; then
    continue
  fi
  if [ ${#licenses[@]} -gt 0 ]; then
    echo ${licenses[@]} | xargs -n 1 | grep -E "^${license}$" > /dev/null
    if [ $? -ne 0 ]; then
      # not found
      continue
    fi
  fi

  # Get version from go.mod or go.sum
  version=`get_package_version $package`

  # Path begins from vendor/...
  path=`echo $fullpath | sed -e "s/^.*\(vendor.*\)$/\1/"`

  # Remove domain of repository in package
  name=`echo $package | sed -e "s/^[^\/]*\/\(.*\)$/\1/"`

  # Generate JSON
  if [ $count -gt 0 ]; then
    echo -n ','
  fi
  count=`expr $count + 1`

  print_dependency_json $name $license $path $version
done

# workaround, go-licenses doesn't return following packages:
# libp2p/go-netroute
# libp2p/go-sockaddr
# pkg/errors
if [ ${#licenses[@]} -gt 0 ]; then
  # BSD-3
  echo ${licenses[@]} | xargs -n 1 | grep -E "^BSD-3-Clause$" > /dev/null
  if [ $? -eq 0 ]; then
    bsd3_dep_packages=("libp2p/go-netroute" "libp2p/go-sockaddr")
    bsd3_dep_paths=("vendor/github.com/libp2p/go-netroute/LICENSE" "vendor/github.com/libp2p/go-netroute/LICENSE")

    i=0
    for package in "${bsd3_dep_packages[@]}"
    do
      path=${bsd3_dep_paths[i]}
      version=`get_package_version $package`
      license="BSD-3-Clause"

      echo -n ','
      print_dependency_json $package $license $path $version

      i=`expr $i + 1`
    done
  fi

  # BSD-2
  echo ${licenses[@]} | xargs -n 1 | grep -E "^BSD-2-Clause$" > /dev/null
  if [ $? -eq 0 ]; then
    bsd2_dep_packages=("pkg/errors")
    bsd2_dep_paths=("vendor/github.com/pkg/errors/LICENSE")

    i=0
    for package in "${bsd2_dep_packages[@]}"
    do
      path=${bsd2_dep_paths[i]}
      version=`get_package_version $package`
      license="BSD-2-Clause"

      echo -n ','
      print_dependency_json $package $license $path $version

      i=`expr $i + 1`
    done
  fi
fi

printf '\n]'
