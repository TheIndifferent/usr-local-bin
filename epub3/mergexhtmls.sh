#!/bin/bash

set -e

filecount="$( ls -1 *'.xhtml' | grep -v 'toc.xhtml' | wc -l )"
totalheight=$((${filecount}*879))

readonly OUTPUT='book.txt'
exec >"${OUTPUT}"

## print header:
echo -n '<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
	<head>
		<meta charset="utf-8" />
		<meta name="viewport" content="width=652,height='
echo -n "${totalheight}"
echo '" />
		<title>Book</title>'
ls -1 'css' | sed -e 's;^;		<link href="css/;g' -e 's;$;" rel="stylesheet" type="text/css" />;g'
echo -n '	</head>
	<body id="Book" style="width:652px;height:'
echo -n "${totalheight}"
echo 'px">'

for file in *'.xhtml'
do
  if [[ "${file}" == 'toc.xhtml' ]]
  then
    continue
  fi
  cat "${file}" | grep '<body' | sed -e 's/body/div/' -e "s/\(style=\"\)\(.*\"\)/\1page-break-after:always;transform:translate(0,0);\2/"
  ## print everything between <body> and </body>:
  cat "${file}" | sed -n -e '1,/<body.*>/d' -e 's/href="\(.*\).xhtml"/href="#\1"/g' -e '/<\/body>/!p;//q'
  echo $'\t</div>'
  # echo $'\t<p style="page-break-after: always;"><br/></p>'
done

echo '	</body>
</html>'

## rename output back to xhtml:
mv "${OUTPUT}" 'book.xhtml'
