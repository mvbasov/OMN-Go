Title: URI schemes test
Date: 2026-08-06 12:00:00
Category: Test
Tags: Test, OMN-Go, OMN-Go user

Split out of [Link test](LinkTest) — link-type tests live there.

#### URI tests
* [tel:+1-555_5555](tel:+1-555-5555)
* [geo: Eiffel Tower z=15](geo:48.8584,2.2945?z=15)
  * z=1: World view
  * z=5: Landmass / Continent level
  * z=10: City level
  * z=15: Neighborhood level
  * z=20: Individual buildings / Roofs
* [geo: Eiffel Tower z=10](geo:48.8584,2.2945?z=10)
* [sms: +1-5555?body=text](sms:+15550199?body=Hello%20there)
* [mailto: name@example.com...](mailto:name@example.com?subject=Subj.&body=Greeting!%0ABody%20here.&cc=cc@example.com&bcc=bcc@example.com)

#### Intent (only Android)
See [Android Intents](../../AndroidIntents) for details

* [Scan a barcode](intent:#Intent;action=com.google.zxing.client.android.SCAN;package=de.markusfisch.android.binaryeye;S.omngo_capture_extra=SCAN_RESULT;end)
* [Capture Kernel info (Termux)](intent:#Intent;action=com.termux.RUN_COMMAND;component=com.termux/.app.RunCommandService;S.com.termux.RUN_COMMAND_LABEL=Kernel%20info;S.com.termux.RUN_COMMAND_PATH=$PREFIX/bin/uname?-a;S.omngo_capture_output=stdout;end)
* [Wireless](intent:#Intent;action=android.settings.WIRELESS_SETTINGS;end)
* [Wi-Fi](intent:#Intent;action=android.settings.WIFI_SETTINGS;end)
* [Bluetooth](intent:#Intent;action=android.settings.BLUETOOTH_SETTINGS;end)
* [NFC](intent:#Intent;action=android.settings.NFC_SETTINGS;end)
* [Location](intent:#Intent;action=android.settings.LOCATION_SOURCE_SETTINGS;end)
* [Device info](intent:#Intent;action=android.settings.DEVICE_INFO_SETTINGS;end)

- - -

<span id="local_counter">...</span>.
<script type="module" src="/js/local_counter.js"></script>
