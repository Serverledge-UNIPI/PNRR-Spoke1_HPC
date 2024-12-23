This repository is a development branch of the open-source Serverledge platform, specifically tailored to implement and evaluate the **Energy-Efficient Function Invocation Scheduling (E²FIS)** policy. 
E²FIS is an optimization-based function scheduling framework designed to minimize energy consumption in Function-as-a-Service (FaaS) platforms deployed in edge-cloud environments, while ensuring that functions meet their execution deadlines.

**Repository Structure**

This branch builds upon the original Serverledge framework by introducing new modules and updates to support E²FIS integration and execution:

	* Solver Module: Implements the Mixed Integer Linear Programming (MILP) model that forms the core of E²FIS. This module computes the optimal scheduling and function placement on worker nodes in the edge zone.
	* Load Balancer: Extends the Serverledge load balancer to enforce the scheduling decisions provided by E²FIS. It also integrates fallback mechanisms for unfeasible solutions.

