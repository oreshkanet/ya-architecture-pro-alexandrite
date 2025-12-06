```
docker build -t service-a:latest -f service-a/Dockerfile .
docker build -t service-b:latest -f service-b/Dockerfile .

docker build -t service-a:latest -f Dockerfile .
docker build -t service-b:latest -f Dockerfile .

kubectl port-forward svc/jaeger-query 16686:16686 -n tracing 
kubectl port-forward svc/service-a 8080:8080 -n tracingcd ..

kubectl apply -f services.yaml  -n tracing   
kubectl delete -f services.yaml  -n tracing   
```