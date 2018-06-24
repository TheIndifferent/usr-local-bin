#!/bin/bash

for file in *'.xhtml'
do
  if [[ -z $file ]]
  then
    continue
  fi
  cssfile="$( cat "${file}" | grep -v 'idGeneratedStyles_0.css' | grep -o 'css/idGeneratedStyles_[0-9]*.css' )"
  if [[ -z $cssfile ]]
  then
    continue
  fi
  css="$( echo "${cssfile}" | sed 's;^css/idGeneratedStyles_\([0-9]*\).css$;\1;' )"
  for style in $( cat "${cssfile}" | grep '^[a-z]' | grep -v '[a-zA-Z]-Disabled ' | sed 's/^[a-z]*\.\(.*\) .*$/\1/g' )
  do
    if [[ -z $style ]]
    then
      continue
    fi
    sed -i '' "s/\([\" ]${style}\)\([\" ]\)/\1-${css}\2/g" "${file}"
  done
done
