TIME_SHORT	= `date +%H:%M:%S`
TIME		= $(TIME_SHORT)

RED          := $(shell printf "\033[31m")
GREEN        := $(shell printf "\033[32m")
CNone        := $(shell printf "\033[0m")

ERR		= echo ${TIME} ${RED}[FAIL]${CNone}
OK		= echo ${TIME} ${GREEN}[ OK ]${CNone}