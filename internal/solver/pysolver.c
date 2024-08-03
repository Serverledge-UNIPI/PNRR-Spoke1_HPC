#ifndef PY_SOLVER_H
#define PY_SOLVER_H

#include <Python.h>
#include <stdio.h>
#include <stdlib.h>

void initializePython() {
    // Initialize the Python interpreter
    Py_Initialize();
    PyRun_SimpleString("import sys; sys.path.insert(0, './internal/solver')");

    // Check if initialization was successful
    if (!Py_IsInitialized()) {
        fprintf(stderr, "Failed to initialize the Python interpreter\n");
    }
}

void finalizePython() {
	if (Py_IsInitialized()) {
		Py_Finalize();
	}
}


// Function to allocate memory for an array of integers
int* allocateMemory(int size) {
    int* array = (int*)malloc(size * sizeof(int));
    if (array == NULL) {
        fprintf(stderr, "Memory allocation failed\n");
        exit(EXIT_FAILURE);
    }
    return array;
}

// Function to free allocated memory
void freeMemory(int* array) {
    free(array);
}

// Function to start the Python solver
const char* startSolver(int numberOfNodes, int numberOfFunctions, int* nodeMemory, int* nodeCapacity, int* maximumCapacity, int* nodeIpc, int* nodePowerConsumption,
                                int* functionMemory, int* functionWorkload, int* functionDeadline, int* functionInvocations) {
    PyObject *pName, *pModule, *pFunc;
    PyObject *pArgs, *pValue;
    const char* result = NULL;

    // Load the Python module
    pName = PyUnicode_DecodeFSDefault("solver_wrapper");
    if (pName == NULL) {
        PyErr_Print();
        fprintf(stderr, "Failed to decode module name\n");
        return NULL;
    }

    pModule = PyImport_Import(pName);
    Py_DECREF(pName);
    
    if (pModule == NULL) {
        PyErr_Print();
        fprintf(stderr, "Failed to load 'solver_wrapper' module\n");
        return NULL;
    }

    // Get the function from the Python module
    pFunc = PyObject_GetAttrString(pModule, "start_solver");
    if (pFunc == NULL || !PyCallable_Check(pFunc)) {
        PyErr_Print();
        fprintf(stderr, "Cannot find or call 'start_solver' function\n");
        Py_DECREF(pModule);
        return NULL;
    }

    // Create the arguments tuple for the Python function
    pArgs = PyTuple_New(11);
    if (pArgs == NULL) {
        PyErr_Print();
        fprintf(stderr, "Failed to create arguments tuple\n");
        Py_DECREF(pFunc);
        Py_DECREF(pModule);
        return NULL;
    }

    // Create lists for arguments and populate them
    PyObject* nodeMemoryList = PyList_New(numberOfNodes);
    PyObject* nodeCapacityList = PyList_New(numberOfNodes);
    PyObject* maximumCapacityList = PyList_New(numberOfNodes);
    PyObject* nodeIpcList = PyList_New(numberOfNodes);
    PyObject* nodePowerConsumptionList = PyList_New(numberOfNodes);

    PyObject* functionMemoryList = PyList_New(numberOfFunctions);
    PyObject* functionWorkloadList = PyList_New(numberOfFunctions);
    PyObject* functionDeadlineList = PyList_New(numberOfFunctions);
    PyObject* functionInvocationsList = PyList_New(numberOfFunctions);

    if (nodeMemoryList == NULL || nodeCapacityList == NULL || maximumCapacityList == NULL || nodeIpcList == NULL || nodePowerConsumptionList == NULL ||
        functionMemoryList == NULL || functionWorkloadList == NULL || functionDeadlineList == NULL || functionInvocationsList == NULL) {
        PyErr_Print();
        fprintf(stderr, "Failed to create lists for arguments\n");
        Py_XDECREF(nodeMemoryList);
        Py_XDECREF(nodeCapacityList);
        Py_XDECREF(maximumCapacityList);
        Py_XDECREF(nodeIpcList);
        Py_XDECREF(nodePowerConsumptionList);
        Py_XDECREF(functionMemoryList);
        Py_XDECREF(functionWorkloadList);
        Py_XDECREF(functionDeadlineList);
        Py_XDECREF(functionInvocationsList);
        Py_DECREF(pArgs);
        Py_DECREF(pFunc);
        Py_DECREF(pModule);
        return NULL;
    }

    // Populate the lists with values
    for (int i = 0; i < numberOfNodes; i++) {
        PyList_SetItem(nodeMemoryList, i, PyLong_FromLong(nodeMemory[i]));
        PyList_SetItem(nodeCapacityList, i, PyLong_FromLong(nodeCapacity[i]));
        PyList_SetItem(maximumCapacityList, i, PyLong_FromLong(maximumCapacity[i]));
        PyList_SetItem(nodeIpcList, i, PyLong_FromLong(nodeIpc[i]));
        PyList_SetItem(nodePowerConsumptionList, i, PyLong_FromLong(nodePowerConsumption[i]));
    }

    for (int j = 0; j < numberOfFunctions; j++) {
        PyList_SetItem(functionMemoryList, j, PyLong_FromLong(functionMemory[j]));
        PyList_SetItem(functionWorkloadList, j, PyLong_FromLong(functionWorkload[j]));
        PyList_SetItem(functionDeadlineList, j, PyLong_FromLong(functionDeadline[j]));
        PyList_SetItem(functionInvocationsList, j, PyLong_FromLong(functionInvocations[j]));
    }

    // Set the arguments in the tuple
    PyTuple_SetItem(pArgs, 0, PyLong_FromLong(numberOfNodes));
    PyTuple_SetItem(pArgs, 1, PyLong_FromLong(numberOfFunctions));
    PyTuple_SetItem(pArgs, 2, nodeMemoryList);
    PyTuple_SetItem(pArgs, 3, nodeCapacityList);
    PyTuple_SetItem(pArgs, 4, maximumCapacityList);
    PyTuple_SetItem(pArgs, 5, nodeIpcList);
    PyTuple_SetItem(pArgs, 6, nodePowerConsumptionList);
    PyTuple_SetItem(pArgs, 7, functionMemoryList);
    PyTuple_SetItem(pArgs, 8, functionWorkloadList);
    PyTuple_SetItem(pArgs, 9, functionDeadlineList);
    PyTuple_SetItem(pArgs, 10, functionInvocationsList);

    // Call the Python function
    pValue = PyObject_CallObject(pFunc, pArgs);
    Py_DECREF(pArgs);

    if (pValue != NULL) {
        if (PyUnicode_Check(pValue)) {
            result = PyUnicode_AsUTF8(pValue);
        } else {
            fprintf(stderr, "Returned PyObject is not a string\n");
        }
        Py_DECREF(pValue);
    } else {
        PyErr_Print();
        fprintf(stderr, "Function call failed\n");
    }

    // Clean up
    Py_DECREF(pFunc);
    Py_DECREF(pModule);

    return result;
}

#endif // PY_SOLVER_H