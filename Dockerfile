FROM centos:8

ARG LOGLEVEL=2
ENV LOGLEVEL=$LOGLEVEL

RUN yum -y update && yum -y install git golang python3

RUN mkdir /go
ENV GOPATH=/go

RUN mkdir /aws-fail2ban
RUN mkdir -p /go/src/github.com/jo-makar/aws-fail2ban

COPY aws.go handler.go jailer.go jailer-service.go logger.go main-service.go table.go /go/src/github.com/jo-makar/aws-fail2ban/

RUN cd /go/src/github.com/jo-makar/aws-fail2ban; go mod init; go build -o /aws-fail2ban

# If specific versions of modules are needed the following can be done instead
#COPY go.mod /go/src/github.com/jo-makar/aws-fail2ban/go.mod
#COPY go.sum /go/src/github.com/jo-makar/aws-fail2ban/go.sum
#RUN cd /go/src/github.com/jo-makar/aws-fail2ban; go mod download; go build -o /aws-fail2ban

RUN pip3 install awscli --upgrade
RUN mkdir /root/.aws; echo -e '[default]\noutput = json\nregion = us-east-1' >/root/.aws/config

WORKDIR /aws-fail2ban
EXPOSE 8000/tcp
CMD ["sh", "-c", "./aws-fail2ban -l $LOGLEVEL -r $REDIS_ADDR $IPSET"]
