include $(GOROOT)/src/Make.inc

TARG=goajax
GOFILES=\
	server.go\

include $(GOROOT)/src/Make.pkg

example:
	$(GC) example.go
	$(LD) -o example example.6
