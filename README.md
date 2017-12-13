# dash
The light-weight server for dashboard's back-end

#### Usage:
1. Make directory struct 
   Dash\Data
2. run command "dash.exe -port=8998 -dsn="MSSQL COnnection String" -debug" from directory "Dash"
3. put query.sql into directory "Dash\Data"
4. insert into your html page script like this
```javascript
$.get( "http://your-server:8998/data/query", function( data ) {
  doSomething(data);
});
```

