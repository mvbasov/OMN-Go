
#zip -r local/current_state_$( date +%Y-%m-%dT%H.%M.%S ).zip -@ <local/to_archive.txt
tar -zcvf local/current_state_$( date +%Y-%m-%dT%H.%M.%S ).tgz -T local/to_archive.txt
