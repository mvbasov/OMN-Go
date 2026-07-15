Title: Console
Date: 2026-06-23 00:21:00
Modified: 2026-07-05 08:12:53
Category: Test
Author: Mihail Basov
Tags: Test

### See console messages
<script type="module">
console.log('--- Start console.time()');
console.time('Label');
const users = [
  { name: 'Alice', role: 'Admin', age: '45' },
  { name: 'Bob', role: 'User', age: '33' }
];
console.log('--- Array:');
console.table(users);
console.log('--- Array but limit columns:');
console.table(users, ['name', 'age']);
const config = { debug: true, port: 3000, status: 'Up' };
console.log('--- Object:');
console.table(config);
console.error('Not real but test error');
console.warn('Not real but test warning');
console.info('Info message');
console.debug('Debug message');
console.log('--- Object as tree:');
console.dir(users); //Hard to implement pn mobile
// For trace test
function app() {
  function doSomething() {
    var a = 1;
    var b = 2;
    sumFunction(a, b);
  }
  function sumFunction(a, b) {
    console.trace("Our First Trace");
    return a + b;
  }
  doSomething();
}
app();
console.log('--- Tie from console.time():');
consoole.timeEnd('Label');
</script>


- - -

<span id="local_counter">...</span>.
<script type="module" src="/js/local_counter.js"></script>
