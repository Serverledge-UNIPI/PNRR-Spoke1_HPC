from sklearn.cluster import KMeans
from sklearn.metrics import silhouette_score
import numpy as np
import json

def handler(params, context):
    try:
        X = np.array(params['X'])

        kmeans = KMeans(n_clusters=20, n_init=40)
        kmeans.fit(X)

        labels = kmeans.labels_
        score = silhouette_score(X, labels)

        return {"Result": json.dumps({"silhouette_score": score})}
    except Exception as e:
        print(e)
        return {"Error": str(e)}