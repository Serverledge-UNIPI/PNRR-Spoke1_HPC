from sklearn.datasets import make_blobs
from sklearn.cluster import KMeans
from sklearn.metrics import silhouette_score
import json
import numpy as np

def handler(params, context):
    try:
        X = np.array(params['X'])

        kmeans = KMeans(n_clusters=10, n_init=40)
        kmeans.fit(X)

        labels = kmeans.labels_
        score = silhouette_score(X, labels)

        result = {"Result": json.dumps({"silhouette_score": score})}
    except Exception as e:
        print(e)
        result = {"Error": str(e)}

    return result