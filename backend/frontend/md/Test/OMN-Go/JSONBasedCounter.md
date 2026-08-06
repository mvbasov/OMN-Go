Title: JSON based counter
Date: 2026-08-05 10:49:24
Category: Test
Tags: JavaScript, JSON, Test, OMN-Go, OMN-Go user


<div id="counterDisplay"></div>

* [Edit /user_json/local-counter.json](/user_json/local-counter.json?edit=true)

<script>
async function incrementServerCounter(filename) {
  const display = document.getElementById('counterDisplay');
  const getUrl = `/user_json/${filename}`;
  const uploadUrl = '/api/upload_json';
  try {
    display.innerHTML += 'Checking counter...<br/>';
    let response = await fetch(getUrl);
    let data;
    if (response.ok) {
      data = await response.json();
    } else if (response.status === 404) {
      data = { counter: 0 };
    } else {
      throw new Error(`GET failed: ${response.status}`);
    }
    display.innerHTML += `Before update: counter = ${data.counter}<br/>`;
    data.counter = (data.counter || 0) + 1;
    const jsonString = JSON.stringify(data, null, 2);
    const file = new File([jsonString], filename, { type: 'application/json' });
    const formData = new FormData();
    formData.append('file', file);
    const uploadResponse = await fetch(uploadUrl, {
      method: 'POST',
      body: formData
    });
    if (!uploadResponse.ok) {
      throw new Error(`Upload failed: ${uploadResponse.status}`);
    }
    display.innerHTML += `After update: counter = ${data.counter}<br/>`;
    return data;
  } catch (error) {
    display.innerHTML += `Error: ${error.message}`;
    throw error;
  }
}
// Usage: call 
incrementServerCounter('local-counter.json');
</script>

- - -

<span id="local_counter">...</span>.
<script type="module" src="/js/local_counter.js"></script>
