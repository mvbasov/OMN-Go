
zip -r local/GoOMN_$( date +%Y-%m-%dT%H.%M.%S ).zip -@ <local/to_archive.txt
tar -cvf local/GoOMN_$( date +%Y-%m-%dT%H.%M.%S )tar -T local/to_archive.txt
