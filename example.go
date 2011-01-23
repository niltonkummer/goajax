package main

import (
	"http"
	"goajax"
	"os"
)

func main() {
	s := goajax.NewServer()
	s.Register(new(Service))

	http.HandleFunc("/", handleIndex)
	http.Handle("/json", s)

	http.ListenAndServe(":9001", nil)

}

func handleIndex(w http.ResponseWriter, r *http.Request) {

	w.Write([]byte(`<html>
<head>
	<title>Json RPC</title>
	<script type="text/javascript" src="http://ajax.googleapis.com/ajax/libs/jquery/1.4.4/jquery.min.js"></script>
	<script type="text/javascript">
	$(function(){
		$('#button').click(function(){
			var a = $('#a').val();
			var b = $('#b').val();
			var body = '{"jsonrpc": "2.0", "method":"Service.Add","params":['+a+', '+b+'],"id":0}';
			$.post("/json", body ,function(data){
				$('#output').html(data.result);
			}, "json");
		});
	});
	</script>
</head>
<body>
	<h1>Go Ajax Example</h1>
	<input id="a" type="text" name="a" style="width: 50px;" value="5" />
	<span>+</span>
	<input id="b" type="text" name="b" style="width: 50px;" value="7" />
	<input id="button" type="button" value="="/>	
	<span id="output"></span>
</body>
</html>`))
}

type Service int

func (s *Service) Add(a, b float64) (float64, os.Error) {
	return a + b, nil
}
