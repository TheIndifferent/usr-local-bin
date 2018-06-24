#!/bin/bash

cd css
for file in *
do
  suffix="$( echo "${file}" | sed 's/[a-zA-Z]*_\([0-9]*\).css/\1/' )"
  sed -i '' "s/^\([^#@]*-[0-9]*\) /\1-${suffix} /g" "${file}"
done
