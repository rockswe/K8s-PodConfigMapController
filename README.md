<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Kubernetes Custom Controller</title>
</head>
<body>

<h1>Kubernetes Custom Controller</h1>
<p>
    This is a Kubernetes controller written in Go that watches for Pod changes and creates ConfigMaps with their metadata.
    The controller supports Pod events, a custom resource definition (CRD) for dynamic behavior, leader election, and Prometheus metrics.
</p>

<h2>Features</h2>
<ul>
    <li>Handles Pod create, delete, and update events.</li>
    <li>Custom Resource Definition (CRD) to control behavior.</li>
    <li>Leader Election for high availability.</li>
    <li>Prometheus metrics for monitoring.</li>
</ul>

<h2>Prerequisites</h2>
<ul>
    <li>Kubernetes cluster and <code>kubectl</code> configured.</li>
    <li>Go 1.19+ installed.</li>
    <li>Prometheus (optional).</li>
</ul>

<h2>Getting Started</h2>

<h3>1. Clone the Repository</h3>
<pre><code>git clone https://github.com/yourusername/go-k8s-controller.git
cd go-k8s-controller</code></pre>

<h3>2. Install Dependencies</h3>
<pre><code>go mod tidy</code></pre>

<h3>3. Apply the CRD</h3>
<pre><code>kubectl apply -f crd/myresource_crd.yaml</code></pre>

<h3>4. Run the Controller</h3>
<pre><code>go run main.go</code></pre>

<h3>5. Deploy a Pod for Testing</h3>
<pre><code>kubectl run test-pod --image=busybox --command -- sleep 3600</code></pre>

<h3>6. Monitor Prometheus Metrics</h3>
<p>Visit <code>http://localhost:8080/metrics</code> for metrics like:</p>
<ul>
    <li><code>pod_create_count</code>: Pods created</li>
    <li><code>pod_delete_count</code>: Pods deleted</li>
</ul>

<h3>7. Use the Custom Resource</h3>
<p>Create a <code>MyResource</code> to control behavior:</p>
<pre><code>kubectl apply -f crd/myresource_example.yaml</code></pre>

<h2>Leader Election</h2>
<p>Leader election ensures that only one controller instance is active in multi-instance setups.</p>
<pre><code>kubectl create deployment controller --image=&lt;your-controller-image&gt; --replicas=3</code></pre>

<h2>Extending the Controller</h2>
<p>The controller can be extended to handle other Kubernetes resources, add custom reconciliation logic, and support more advanced CRD behaviors.</p>

<h2>License</h2>
<p>Licensed under the MIT License.</p>

</body>
</html>
