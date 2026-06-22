Title: Console
Date: 2026-06-23 00:21:00
Modified: 2026-06-23 00:29:18
Category: Test
Author: Mihail Basov
Tags: Test

<script type="module">
const users = [
  { name: 'Alice', role: 'Admin', age: '45' },
  { name: 'Bob', role: 'User', age: '33' }
];
console.log('Array:');
console.table(users);
console.log('Array but limit columns:');
console.table(users, ['name', 'age']);
const config = { debug: true, port: 3000, status: 'Up' };
console.log('Object:');
console.table(config);
</script>
